FROM registry.access.redhat.com/ubi8/go-toolset:1.15.7 as build
WORKDIR /opt/app-root
COPY . .
RUN go build

FROM registry.access.redhat.com/ubi8/ubi-minimal
WORKDIR /opt/app-root
COPY --from=build /opt/app-root/openshift-build-annotate /opt/app-root/openshift-build-annotate

ENTRYPOINT ["./openshift-build-annotate"]