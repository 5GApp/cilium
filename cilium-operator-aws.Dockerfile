# (first line comment needed for DOCKER_BUILDKIT use)
#
ARG BASE_IMAGE=scratch

FROM docker.io/library/golang:1.15.2 as builder
ARG CILIUM_SHA=""
LABEL cilium-sha=${CILIUM_SHA}
LABEL maintainer="maintainer@cilium.io"

ADD . /go/src/github.com/cilium/cilium

WORKDIR /go/src/github.com/cilium/cilium/operator
ARG NOSTRIP
ARG LOCKDEBUG
RUN make NOSTRIP=$NOSTRIP LOCKDEBUG=$LOCKDEBUG cilium-operator-aws

FROM docker.io/library/alpine:3.9.3 as certs
ARG CILIUM_SHA=""
LABEL cilium-sha=${CILIUM_SHA}
RUN apk --update add ca-certificates

FROM docker.io/library/golang:1.15.2 as gops
ARG CILIUM_SHA=""
LABEL cilium-sha=${CILIUM_SHA}
RUN go get -d github.com/google/gops && \
    cd /go/src/github.com/google/gops && \
    git checkout -b v0.3.10 v0.3.10 && \
    git --no-pager remote -v && \
    git --no-pager log -1 && \
    CGO_ENABLED=0 go install && \
    strip /go/bin/gops

FROM ${BASE_IMAGE}
ARG CILIUM_SHA=""
LABEL cilium-sha=${CILIUM_SHA}
LABEL maintainer="maintainer@cilium.io"
COPY --from=builder /go/src/github.com/cilium/cilium/operator/cilium-operator-aws /usr/bin/cilium-operator-aws
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=gops /go/bin/gops /bin/gops
WORKDIR /
CMD ["/usr/bin/cilium-operator-aws"]
