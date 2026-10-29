FROM node:24-bookworm-slim
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends systemd systemd-sysv dbus libpam-systemd && rm -rf /var/lib/apt/lists/*
RUN printf 'gator disposable boot fixture\n' > /gator-boot-fixture
ENV container=docker
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
