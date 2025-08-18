FROM registry.ci.openshift.org/ci/origin-release:golang-1.23
WORKDIR /go/src/github.com/openshift/content-mirror
COPY . .
RUN make build

FROM registry.ci.openshift.org/openshift/centos:stream9
COPY --from=0 /go/src/github.com/openshift/content-mirror/content-mirror /usr/bin/content-mirror
COPY nginx.repo /etc/yum.repos.d/nginx.repo
RUN INSTALL_PKGS=" \
      nginx \
      " && \
    yum install -y ${INSTALL_PKGS} && rpm -V ${INSTALL_PKGS} && \
    yum clean all && \
    rm -rf /var/lib/rpm /var/lib/yum/history && \
    mkdir -p /var/cache/nginx && \
    chmod -R uga+rwx /var/cache/nginx /var/log/nginx /run
USER 1001
