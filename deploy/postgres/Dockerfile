FROM postgres:16-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777

# PostgreSQL calls archive_command inside this image; the archive helper needs GPG.
RUN apk add --no-cache gnupg
