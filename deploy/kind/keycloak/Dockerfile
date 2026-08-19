FROM quay.io/keycloak/keycloak:26.2@sha256:e1bea10dc9a144b0f70aba477ba2ef14a0fa752e7e95faa4b7903ffa1287187a AS builder

RUN /opt/keycloak/bin/kc.sh build \
    --db=dev-file \
    --health-enabled=true

FROM quay.io/keycloak/keycloak:26.2@sha256:e1bea10dc9a144b0f70aba477ba2ef14a0fa752e7e95faa4b7903ffa1287187a

COPY --from=builder /opt/keycloak/ /opt/keycloak/

ENTRYPOINT ["/opt/keycloak/bin/kc.sh"]
