# Google Cloud VM Deployment

This repo deploys cleanly to a single Ubuntu VM with Docker and Docker Compose.

The deployment shape is:

- Nginx serves the frontend on port `80`.
- Nginx proxies `/api/*` to the backend container on the internal Docker network.
- The backend talks to Postgres over the internal Docker network.
- The API is bound to `127.0.0.1:8080` on the VM so it is not exposed publicly.

## VM prerequisites

- Ubuntu 22.04 or 24.04.
- A public external IP.
- Firewall rule allowing inbound `tcp:80` to the VM.
- SSH access to the VM.

## Install Docker

On the VM:

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates curl gnupg
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker "$USER"
newgrp docker
docker version
docker compose version
```

## Deploy

On the VM:

```bash
git clone https://github.com/rubber-duck-coder/lumalabs-eng-take-home-ninad.git
cd lumalabs-eng-take-home-ninad
cp .env.example .env
```

Edit `.env` on the VM so the web container binds to port `80`:

```bash
WEB_PORT=80
API_PORT=8080
```

Then start the stack:

```bash
docker compose up --build -d
docker compose ps
```

## Verify

Check health and API routing from the VM:

```bash
curl -sf http://127.0.0.1/api/health
curl -sf http://127.0.0.1/api/nodes
```

Check the public frontend from your laptop:

```bash
curl -sf http://<vm-external-ip>/
curl -sf http://<vm-external-ip>/api/health
```

Run the live E2E suite against the deployed URL:

```bash
BASE_URL=http://<vm-external-ip> make e2e
```

## Updates

To roll a new version:

```bash
git pull
docker compose up --build -d
```

## Optional: Auto-Deploy From GitHub Actions

Recommended secret layout:

- Create a GitHub Environment named `production`.
- Store all deploy secrets in that environment, not as plain repo-level secrets.
- Protect the environment with required reviewers if you want an approval gate.

Secrets needed:

- `VM_SSH_HOST`
- `VM_SSH_USER`
- `VM_SSH_PRIVATE_KEY`
- `VM_SSH_KNOWN_HOSTS`
- `VM_SSH_PORT` optional, defaults to `22`

Step by step:

1. Create a dedicated deploy user on the VM:

```bash
sudo adduser deploy
sudo usermod -aG docker deploy
```

2. Generate a dedicated key pair locally:

```bash
ssh-keygen -t ed25519 -C "github-actions-vm-deploy" -f ./vm_deploy_key -N ""
```

3. Add the public key to the deploy user on the VM:

```bash
ssh deploy@34.105.88.70 'mkdir -p ~/.ssh && chmod 700 ~/.ssh'
cat ./vm_deploy_key.pub | ssh deploy@34.105.88.70 'cat >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys'
```

4. Capture the VM host key and verify it before storing it:

```bash
ssh-keyscan -H 34.105.88.70
```

Use the output from that command as the `VM_SSH_KNOWN_HOSTS` secret. If you want to be stricter, compare it with the fingerprint shown by an interactive SSH connection first.

5. In GitHub:
- Go to repo `Settings`
- Open `Environments`
- Create `production`
- Add the secrets above

6. Trigger the workflow:
- Push to `main`, or
- Use the `workflow_dispatch` button in GitHub Actions

7. On the VM, make sure the deploy user can start the stack once manually:

```bash
sudo -u deploy -H bash -lc 'cd ~/lumalabs-eng-take-home-ninad && docker compose up -d'
```

## Notes

- The frontend is built to use `/api` as its base path, so the same container image works locally and on the VM.
- The local developer stack still works with `docker compose up --build` and the same `/api` path.
- If you want to expose HTTPS later, terminate TLS in front of the Nginx container and keep the same backend and compose layout.
- If you rotate the VM’s external IP, update `VM_SSH_HOST` and regenerate `VM_SSH_KNOWN_HOSTS`.
