#!/usr/bin/env bash
# Copy the current working source into independent local repositories. No history rewrite.
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
output=''
while (($#)); do
  case "$1" in
    --output) [[ $# -ge 2 && -n "$2" ]] || { echo '--output requires a directory' >&2; exit 2; }; output=$2; shift 2 ;;
    --dry-run) output=''; shift ;;
    -h|--help) echo 'Usage: scripts/split-repositories.sh [--output NEW_DIRECTORY]'; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; exit 2 ;;
  esac
done
if [[ -z "$output" ]]; then
  echo "Dry run: frontend/ -> videoflow-web; backend, API, infrastructure -> videoflow-api."
  echo 'No files changed. Use --output NEW_DIRECTORY to create local repositories.'
  exit 0
fi
command -v git >/dev/null
command -v python3 >/dev/null
# Exclusive mkdir rejects existing directories and prevents accidental overwrite.
mkdir -- "$output"
output=$(cd -- "$output" && pwd -P)
case "$output/" in "$root/"*) rmdir -- "$output"; echo 'Output must be outside the source repository.' >&2; exit 2 ;; esac
python3 - "$root" "$output" <<'PY'
from pathlib import Path
import shutil, sys, json
src, dst = map(Path, sys.argv[1:])
ignored = {'.git','.codegraph','.terraform','node_modules','.svelte-kit','build','dist','storage','data','__pycache__','.venv'}
def ignore(directory, names):
    return [n for n in names if n in ignored or n.startswith('.env') and n not in {'.env.example','.env.cloud.example'} or n.endswith(('.tfstate','.tfstate.backup','.tfplan','.pem','.key')) or n.endswith('.tfvars') and not n.endswith('.tfvars.example')]
shutil.copytree(src/'frontend', dst/'videoflow-web', ignore=ignore, symlinks=True)
web=dst/'videoflow-web'
if (src/'docs/launch').is_dir():
    shutil.copytree(src/'docs/launch', web/'docs/launch', dirs_exist_ok=True)
package=json.loads((web/'package.json').read_text())
for name in ('dev','build','check'):
    package['scripts'][name]='VITE_CLOUD_MODE=true '+package['scripts'][name]
(web/'package.json').write_text(json.dumps(package,indent=2)+'\n')
(web/'.gitignore').write_text('node_modules/\n.svelte-kit/\nbuild/\n.env\n.env.*\n!.env.example\n')
(web/'README.md').write_text('# VideoFlow Web\n\nSvelteKit + shadcn-svelte SaaS frontend. Run `npm ci`, then `CLOUD_API_INTERNAL_URL=http://localhost:8000/v1 npm run dev`. Cloud mode is enabled by the package scripts. Production: `docker build -t videoflow-web .`; run with `CLOUD_API_INTERNAL_URL`, `ORIGIN`, and `PORT`. The backend API contract lives in the companion videoflow-api repository; generated wire types are included. No provider or payment secrets belong here.\n')
backend=dst/'videoflow-api'; backend.mkdir()
for name in ('cmd','internal','pkg','api','migrations','infra','scripts','docs','deploy','go.mod','go.sum','Makefile','README.md','LICENSE','.gitignore','.dockerignore','.env.cloud.example','compose.cloud.yml'):
    source=src/name
    if source.is_dir(): shutil.copytree(source,backend/name,ignore=ignore,symlinks=True)
    elif source.is_file(): shutil.copy2(source,backend/name)
# Keep the backend contract's shared wire types alongside OpenAPI.
shutil.copy2(src/'frontend/src/lib/cloud/generated.ts', backend/'api/generated.ts')
if not (dst/'videoflow-web'/'.dockerignore').exists():
    (dst/'videoflow-web'/'.dockerignore').write_text('.git\n.env*\nnode_modules\n.svelte-kit\nbuild\n')
web_dockerfile=src/'deploy/Dockerfile.cloud-web'
if web_dockerfile.is_file():
    (dst/'videoflow-web'/'Dockerfile').write_text(web_dockerfile.read_text().replace('COPY frontend/package*.json ./','COPY package*.json ./').replace('COPY frontend/ ./','COPY . ./'))
workflow=src/'.github/workflows/cloud.yml'
if workflow.is_file():
    for repo in (dst/'videoflow-web',backend):
        target=repo/'.github/workflows'
        target.mkdir(parents=True,exist_ok=True)
        shutil.copy2(workflow,target/'cloud.yml')
compose=(backend/'compose.cloud.yml').read_text().replace('dockerfile: deploy/Dockerfile.cloud-web','dockerfile: Dockerfile')
compose=compose.replace('context: .\n      dockerfile: Dockerfile','context: ../videoflow-web\n      dockerfile: Dockerfile')
(backend/'compose.cloud.yml').write_text(compose)
(backend/'EXTRACTION.md').write_text('Source snapshot extraction; frontend and desktop sources remain in the original repository. Cloud compose builds the sibling ../videoflow-web repository. Backend commands and Terraform build from this repository root. No history was rewritten.\n')
PY
for repo in videoflow-web videoflow-api; do
  git -C "$output/$repo" init --quiet --initial-branch=main
  echo "Created $output/$repo (uncommitted source snapshot)"
done
echo 'Original working tree preserved; no commits, remotes, or pushes performed.'
