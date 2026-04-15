# syntax=docker/dockerfile:1
FROM scratch
ARG TARGETPLATFORM
USER 65534:65534  # nobody:nogroup
COPY --chown=root:root --chmod=555 $TARGETPLATFORM/lostromos /lostromos
ENTRYPOINT ["/lostromos"]
