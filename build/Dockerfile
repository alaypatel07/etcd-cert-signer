FROM registry.access.redhat.com/ubi7/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/etcd-cert-signer \
    USER_UID=1001 \
    USER_NAME=etcd-cert-signer

# install operator binary
COPY build/_output/bin/etcd-cert-signer ${OPERATOR}

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
