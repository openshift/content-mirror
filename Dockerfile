FROM openshift/origin-release:golang-1.9
COPY . /go/src/github.com/openshift/content-mirror/
RUN GOPATH=/go go install github.com/openshift/content-mirror/cmd/content-mirror

FROM centos:7
COPY --from=0 /go/bin/content-mirror /usr/bin/content-mirror
COPY nginx.repo /etc/yum.repos.d/nginx.repo
# Note: nginx creates "/var/run/nginx.pid". Runtime user needs to be able to create this file.
RUN INSTALL_PKGS=" \
      nginx \
      " && \
    yum install --enablerepo=nginx -y ${INSTALL_PKGS} && rpm -V ${INSTALL_PKGS} && \
    yum clean all && \
    rm -rf /var/lib/rpm /var/lib/yum/history && \
    chmod -R uga+rwx /var/cache/nginx /var/log/nginx && \
    chmod uga+rwx /var/run
USER 1001