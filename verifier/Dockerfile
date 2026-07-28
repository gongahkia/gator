FROM mcr.microsoft.com/playwright:v1.62.0-noble
WORKDIR /opt/norbot-verifier
COPY package.json package-lock.json ./
RUN npm ci --omit=dev && groupadd --gid 10001 norbot && useradd --uid 10001 --gid 10001 --create-home norbot
USER 10001:10001
