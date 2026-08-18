FROM quay.io/keycloak/keycloak:26.2 AS builder

RUN /opt/keycloak/bin/kc.sh build \
    --db=dev-file \
    --health-enabled=true

FROM quay.io/keycloak/keycloak:26.2

COPY --from=builder /opt/keycloak/ /opt/keycloak/

ENTRYPOINT ["/opt/keycloak/bin/kc.sh"]
