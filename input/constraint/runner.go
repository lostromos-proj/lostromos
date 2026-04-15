package constraint

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/lostromos-proj/lostromos/metrics"
)

const (
	templateGroup    = "templates.gatekeeper.sh"
	templateVersion  = "v1"
	templateResource = "constrainttemplates"

	constraintGroup   = "constraints.gatekeeper.sh"
	constraintVersion = "v1beta1"

	initialSyncTimeout = 30 * time.Second
	backoffInitial     = time.Second
	backoffMax         = 5 * time.Minute
)

// violationKey uniquely identifies a violation within a constraint.
type violationKey struct {
	enforcementAction, namespace, kind, name string
}

// kindState holds the per-kind watch state.
type kindState struct {
	kind     string
	stopOnce sync.Once
	stop     chan struct{}
	mu       sync.Mutex
	active   map[string]map[violationKey]struct{} // constraintName -> set of active violations
}

func newKindState(kind string) *kindState {
	return &kindState{
		kind:   kind,
		stop:   make(chan struct{}),
		active: make(map[string]map[violationKey]struct{}),
	}
}

func (ks *kindState) doStop() {
	ks.stopOnce.Do(func() { close(ks.stop) })
}

// Runner watches ConstraintTemplates and sets up per-kind constraint informers.
type Runner struct {
	dynamicClient dynamic.Interface
	updates       chan<- metrics.Update

	runCtx context.Context // set by Run before goroutines are launched

	mu    sync.Mutex
	kinds map[string]*kindState // template name -> kindState
}

// NewRunner creates a Runner that will publish violation updates to updates.
func NewRunner(dynamicClient dynamic.Interface, updates chan<- metrics.Update) *Runner {
	return &Runner{
		dynamicClient: dynamicClient,
		updates:       updates,
		kinds:         make(map[string]*kindState),
	}
}

// Run starts the ConstraintTemplate informer and blocks until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) error {
	r.runCtx = ctx

	factory := dynamicinformer.NewDynamicSharedInformerFactory(r.dynamicClient, 0)
	templateGVR := schema.GroupVersionResource{
		Group:    templateGroup,
		Version:  templateVersion,
		Resource: templateResource,
	}

	inf := factory.ForResource(templateGVR).Informer()
	if _, err := inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.onTemplateAdd,
		UpdateFunc: r.onTemplateUpdate,
		DeleteFunc: r.onTemplateDelete,
	}); err != nil {
		return fmt.Errorf("add constrainttemplate event handler: %w", err)
	}

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
		if err := ctx.Err(); err != nil {
			return err
		}
		return fmt.Errorf("timed out waiting for constrainttemplate cache sync")
	}

	<-ctx.Done()

	// Stop all active kind watches.
	r.mu.Lock()
	allKinds := make([]*kindState, 0, len(r.kinds))
	for _, ks := range r.kinds {
		allKinds = append(allKinds, ks)
	}
	r.mu.Unlock()

	for _, ks := range allKinds {
		ks.doStop()
	}
	return nil
}

func (r *Runner) onTemplateAdd(obj any) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	templateName := u.GetName()
	kind := extractTemplateKind(u)
	if kind == "" {
		slog.Warn("constrainttemplate has no kind", "template", templateName)
		return
	}

	r.mu.Lock()
	if _, exists := r.kinds[templateName]; exists {
		r.mu.Unlock()
		return
	}
	ks := newKindState(kind)
	r.kinds[templateName] = ks
	r.mu.Unlock()

	go r.watchKindWithBackoff(templateName, kind, ks)
}

// onTemplateUpdate handles kind changes. The informer guarantees AddFunc fires
// before UpdateFunc, so r.kinds[templateName] is always set on entry.
func (r *Runner) onTemplateUpdate(_, newObj any) {
	u, ok := newObj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	templateName := u.GetName()
	newKind := extractTemplateKind(u)
	if newKind == "" {
		return
	}

	r.mu.Lock()
	ks := r.kinds[templateName]
	if ks == nil || ks.kind == newKind {
		r.mu.Unlock()
		return
	}
	newKs := newKindState(newKind)
	r.kinds[templateName] = newKs
	r.mu.Unlock()

	r.teardownKind(ks)
	go r.watchKindWithBackoff(templateName, newKind, newKs)
}

func (r *Runner) onTemplateDelete(obj any) {
	u, ok := toUnstructured(obj)
	if !ok {
		return
	}
	templateName := u.GetName()

	r.mu.Lock()
	ks, exists := r.kinds[templateName]
	if !exists {
		r.mu.Unlock()
		return
	}
	delete(r.kinds, templateName)
	r.mu.Unlock()

	r.teardownKind(ks)
}

// teardownKind stops a kind watch and sends active=false for all tracked violations.
func (r *Runner) teardownKind(ks *kindState) {
	ks.doStop()
	ks.mu.Lock()
	defer ks.mu.Unlock()
	for constraintName, violations := range ks.active {
		for vk := range violations {
			r.updates <- metrics.Update{
				Source:             "constraint",
				Policy:             constraintName,
				Decision:           vk.enforcementAction,
				ViolationNamespace: vk.namespace,
				Kind:               vk.kind,
				Name:               vk.name,
				Active:             false,
			}
		}
	}
}

// watchKindWithBackoff retries setting up the informer for kind with exponential backoff.
func (r *Runner) watchKindWithBackoff(templateName, kind string, ks *kindState) {
	ctx := r.runCtx
	backoff := backoffInitial
	for {
		// Bail out if this kindState is no longer current (template removed/updated).
		r.mu.Lock()
		current, ok := r.kinds[templateName]
		r.mu.Unlock()
		if !ok || current != ks {
			return
		}

		err := r.runKindInformer(ctx, kind, ks)
		if err == nil {
			return // clean shutdown
		}

		slog.Warn("constraint kind watch failed", "kind", kind, "err", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-ks.stop:
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, backoffMax)
	}
}

// runKindInformer sets up and runs an informer for the given constraint kind.
// Returns nil on clean shutdown, error if setup or sync fails.
func (r *Runner) runKindInformer(ctx context.Context, kind string, ks *kindState) error {
	gvr := schema.GroupVersionResource{
		Group:    constraintGroup,
		Version:  constraintVersion,
		Resource: strings.ToLower(kind),
	}

	factory := dynamicinformer.NewDynamicSharedInformerFactory(r.dynamicClient, 0)
	inf := factory.ForResource(gvr).Informer()

	if _, err := inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			r.onConstraintChange(ks, obj)
		},
		UpdateFunc: func(_, newObj any) {
			r.onConstraintChange(ks, newObj)
		},
		DeleteFunc: func(obj any) {
			r.onConstraintDelete(ks, obj)
		},
	}); err != nil {
		return err
	}

	stop := make(chan struct{})
	factory.Start(stop)

	// Wait for initial cache sync; timeout detects CRD-not-yet-available.
	syncCtx, syncCancel := context.WithTimeout(ctx, initialSyncTimeout)
	defer syncCancel()
	if !cache.WaitForCacheSync(syncCtx.Done(), inf.HasSynced) {
		close(stop)
		return fmt.Errorf("%s cache sync timed out (CRD may not be available yet)", kind)
	}

	slog.Debug("constraint kind watch established", "kind", kind)

	select {
	case <-ctx.Done():
	case <-ks.stop:
	}
	close(stop)
	return nil
}

func (r *Runner) onConstraintChange(ks *kindState, obj any) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	constraintName := u.GetName()
	newViolations := extractViolations(u)

	ks.mu.Lock()
	old := ks.active[constraintName]
	ks.active[constraintName] = newViolations
	ks.mu.Unlock()

	r.diffAndSend(constraintName, old, newViolations)
}

func (r *Runner) onConstraintDelete(ks *kindState, obj any) {
	u, ok := toUnstructured(obj)
	if !ok {
		return
	}
	constraintName := u.GetName()

	ks.mu.Lock()
	old := ks.active[constraintName]
	delete(ks.active, constraintName)
	ks.mu.Unlock()

	r.diffAndSend(constraintName, old, nil)
}

// diffAndSend sends active=false for violations removed and active=true for violations added.
func (r *Runner) diffAndSend(constraintName string, old, new map[violationKey]struct{}) {
	for vk := range old {
		if _, ok := new[vk]; !ok {
			r.updates <- metrics.Update{
				Source:             "constraint",
				Policy:             constraintName,
				Decision:           vk.enforcementAction,
				ViolationNamespace: vk.namespace,
				Kind:               vk.kind,
				Name:               vk.name,
				Active:             false,
			}
		}
	}
	for vk := range new {
		if _, ok := old[vk]; !ok {
			r.updates <- metrics.Update{
				Source:             "constraint",
				Policy:             constraintName,
				Decision:           vk.enforcementAction,
				ViolationNamespace: vk.namespace,
				Kind:               vk.kind,
				Name:               vk.name,
				Active:             true,
			}
		}
	}
}

func extractViolations(u *unstructured.Unstructured) map[violationKey]struct{} {
	result := make(map[violationKey]struct{})
	violList, _, _ := unstructured.NestedSlice(u.Object, "status", "violations")
	for _, v := range violList {
		vm, ok := v.(map[string]any)
		if !ok {
			continue
		}
		vk := violationKey{
			enforcementAction: strField(vm, "enforcementAction"),
			namespace:         strField(vm, "namespace"),
			kind:              strField(vm, "kind"),
			name:              strField(vm, "name"),
		}
		if vk.name != "" {
			result[vk] = struct{}{}
		}
	}
	return result
}

func extractTemplateKind(u *unstructured.Unstructured) string {
	kind, _, _ := unstructured.NestedString(u.Object, "spec", "crd", "spec", "names", "kind")
	return kind
}

// toUnstructured extracts an *unstructured.Unstructured from a delete event object,
// handling both direct objects and DeletedFinalStateUnknown tombstones.
func toUnstructured(obj any) (*unstructured.Unstructured, bool) {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		return u, true
	}
	if ts, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		u, ok := ts.Obj.(*unstructured.Unstructured)
		return u, ok
	}
	return nil, false
}

func strField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
