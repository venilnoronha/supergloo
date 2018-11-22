FROM alpine

RUN apk -U add ca-certificates

RUN mkdir /tmp/helm && cd /tmp/helm && \
    wget https://storage.googleapis.com/kubernetes-helm/helm-v2.11.0-linux-amd64.tar.gz && \
    tar xzf helm-*.tar.gz && cp linux-amd64/helm /bin/  && \
    rm -rf /tmp/helm

RUN helm init --client-only

COPY supergloo-linux-amd64 /usr/local/bin/supergloo

ENTRYPOINT ["/usr/local/bin/supergloo"]