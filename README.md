# Local models with Ollama, OpenCode, and GitHub Copilot

Download only the models you want, then select them in GitHub Copilot,
OpenCode, Open WebUI, or Continue in VS Code. The defaults target a single RTX 5090 with 32 GB VRAM:
one loaded model, one concurrent request, and an 80K context window.

The stack pins container versions:
[Ollama 0.40.1](https://github.com/ollama/ollama/releases/tag/v0.40.1) and
[Open WebUI v0.11.4](https://github.com/open-webui/open-webui/releases/tag/v0.11.4).
They do not follow floating `latest` or `main` tags.

## Native Windows setup

Use a data drive for model storage, for example `I:\var\lib\ollama\models`.
The Ollama engine listens on `127.0.0.1:11435`; a native build of the existing
compatibility service listens on `127.0.0.1:11434`. Copilot, Continue, and
OpenCode should use `http://localhost:11434/v1` for OpenAI-compatible requests.
Native Ollama APIs also pass through that endpoint unchanged.
Docker is not required for either process.

Persist the runtime profile in your Windows user environment, adjusting the
model directory if necessary:

```powershell
$profile = @{
    OLLAMA_MODELS = 'I:\var\lib\ollama\models'
    OLLAMA_HOST = 'http://127.0.0.1:11435'
    OLLAMA_CONTEXT_LENGTH = '81920'
    OLLAMA_KEEP_ALIVE = '30m'
    OLLAMA_LOAD_TIMEOUT = '10m'
    OLLAMA_MAX_LOADED_MODELS = '1'
    OLLAMA_NUM_PARALLEL = '1'
}
foreach ($entry in $profile.GetEnumerator()) {
    [Environment]::SetEnvironmentVariable($entry.Key, $entry.Value, 'User')
    [Environment]::SetEnvironmentVariable($entry.Key, $entry.Value, 'Process')
}
```

Quit and restart Ollama after changing environment variables. Its saved
Desktop context setting takes precedence over `OLLAMA_CONTEXT_LENGTH`; keep
that override unset or consistent with the **81920-token** profile. A 32K or 64K Desktop
override is too small for the Copilot budget below. Leave GPU backend
selection automatic: the native installation uses CUDA without the container's
`cuda_v12` override.

`Modelfile.qwen38` sets an 8192-token generation default. Context remains a
server setting so changing the runtime profile does not require rebuilding
models. Recreate the alias after changing that file:

```powershell
$source = 'hf.co/huihui-ai/Huihui-Qwen3.8-27B-abliterated-GGUF:Huihui-Qwen3.8-27B-abliterated-Q6_K.gguf'
ollama pull $source
ollama create qwen38 -f .\Modelfile.qwen38
ollama rm $source
```

The source tag is removed after creating `qwen38` to avoid a duplicate picker
entry; its weights remain referenced by `qwen38`.

Model defaults can be overridden by requests. The compatibility endpoint
additionally enforces the Qwen Chat Completions output cap and defaults to medium
reasoning, including when a client requests unsupported `high` reasoning.
Explicit low reasoning and disabled thinking (`none`) are preserved.
Keep Copilot's prompt/output budgets at **65536 / 8192** and its supported
reasoning effort at **medium**.

To rebuild the native proxy, stop its existing process first, then run:

```powershell
New-Item -ItemType Directory -Force I:\opt\ollama, I:\var\log\ollama
go -C .\compat build '-ldflags=-H=windowsgui' -o I:\opt\ollama\ollama-compat.exe .
& I:\opt\ollama\ollama-compat.exe `
    -listen 127.0.0.1:11434 `
    -upstream http://127.0.0.1:11435 `
    -log-file I:\var\log\ollama\compat.log
```

The Windows GUI build avoids opening a console window; its log file retains
startup and request errors. To start the proxy at sign-in, add a shortcut to
the executable in the current user's Startup folder, using the same arguments.
Keep that shortcut enabled alongside Ollama's when using port 11434.
The native port 11435 bypasses compatibility adaptations.

Changing model directories does not migrate files. Preserve complete model
stores, including `blobs`, `manifests`, `manifests-v2`, and `metadata`; retain
the original stores until discovery and inference have been verified.

## Start the Docker stack

Use Docker Desktop with Linux containers, WSL2, and NVIDIA GPU support on
Windows (or Docker with the NVIDIA Container Toolkit on Linux).

```powershell
Copy-Item .env.example .env
New-Item -ItemType Directory -Force F:\opt\docker\ollama, F:\opt\docker\open-webui
docker compose pull
docker compose up -d --build
```

Edit `.env` first if your data lives elsewhere, and create those directories.
The default storage directory is `F:\opt\docker`.
Changing the data paths selects different storage; it does not migrate existing data.
Keep `COMPOSE_PROJECT_NAME` consistent with your existing installation to reuse
its volumes. `.env.example` uses `ollama`; without `.env`, Compose uses the
directory name. Do not use `docker compose down -v` to switch models.

Open [Open WebUI](http://localhost:3000) and create your local account.
Ollama's API is at `http://localhost:11434` through the local compatibility
service. Both ports are bound to localhost. Open WebUI connects directly to
Ollama inside the Compose network.
The first startup downloads container images, **not model weights**.
Do not run the Docker and native setups together: both compatibility endpoints
use port 11434, and they should not write to the same model store concurrently.

## Pick models

```powershell
.\pull.ps1 -List              # Show choices without Docker or a download
.\pull.ps1                    # Interactive picker; accepts qwen or qwen,dolphin
.\pull.ps1 qwen               # Download and configure the recommended Qwen model
.\pull.ps1 qwen,dolphin,hermes # Download several, sequentially
.\pull.ps1 hermes -Run        # Download and start an interactive terminal chat
```

If script execution is disabled, use
`pwsh -NoProfile -ExecutionPolicy Bypass -File .\pull.ps1` for this invocation
only. The picker reads `config.json`, so there is one model list to maintain.
You can also pass an exact model ID from that file to `-Model`.

| Name | Model | Quant | Download size | Purpose |
|---|-------|-------|---------------|---------|
| qwen | [Qwen3.8 27B abliterated](https://huggingface.co/huihui-ai/Huihui-Qwen3.8-27B-abliterated-GGUF) | Q6_K | 22.4 GB | General-purpose starting point (`qwen38` in Ollama) |
| qwen-moe | [Qwen3.6 35B-A3B abliterated](https://huggingface.co/huihui-ai/Huihui-Qwen3.6-35B-A3B-abliterated-MTP-GGUF) | Q5_K | 25.3 GB | MoE alternative; compare speed and quality locally |
| dolphin | [Dolphin3.0 Mistral 24B](https://huggingface.co/mradermacher/Dolphin3.0-Mistral-24B-GGUF) | Q6_K | 19.3 GB | Refusal-reduced fine-tune alternative |
| hermes | [Hermes 4 14B](https://huggingface.co/bartowski/NousResearch_Hermes-4-14B-GGUF) | Q6_K | 12.1 GB | Smaller, steerable alternative |
| coder | Qwen2.5 Coder 7B | Ollama default | About 5 GB | Coding/autocomplete model (not uncensored) |

Sizes are weight files, **not total VRAM usage**. Context, runtime buffers, and
the display consume additional memory. The two Qwen entries are abliterated;
Dolphin and Hermes use different training approaches. No model is guaranteed
never to refuse, and reduced refusals do not mean better accuracy.

The exact GGUF filenames are pinned in the model IDs using
[Hugging Face's Ollama filename-tag syntax](https://huggingface.co/docs/hub/ollama).
This avoids ambiguous default quantization or accidentally selecting a
specialized ternary build. Use current Ollama images for these architectures;
an available GGUF is not proof that every Ollama version can execute it.
This setup uses text chat and does not configure vision projectors or promise
MTP speculative decoding.

The `qwen` choice creates the short Ollama name `qwen38` using
`Modelfile.qwen38`. It provides a Go chat template for chat history and tools.
The GGUF's reasoning template accepts only `xhigh`, `medium`, or `low`.
The compatibility service defaults Qwen Chat Completions to **medium**, preserves
explicit **low** and **none**, and caps generation at 8192 tokens, including reasoning. The Ollama container
uses the CUDA 12 backend for GPU inference. Verify `ollama ps` reports
`100% GPU` after a request.

On Bash/Linux/macOS, pass model IDs explicitly (any Ollama-compatible ID works):

```bash
bash pull.sh 'hf.co/huihui-ai/Huihui-Qwen3.8-27B-abliterated-GGUF:Huihui-Qwen3.8-27B-abliterated-Q6_K.gguf'
bash pull.sh qwen2.5-coder:7b deepseek-r1:14b
```

## OpenCode Desktop and CLI

`opencode.json` configures **OpenCode 2.1.2** to use the local abliterated
`qwen38:latest` model through the compatibility endpoint. To use it in other
projects, merge its settings into `%USERPROFILE%\.config\opencode\opencode.json`
and install the dependency-free summary hook:

```powershell
$plugins = Join-Path $HOME '.config\opencode\plugins'
New-Item -ItemType Directory -Force $plugins
Copy-Item .\opencode-compaction.js (Join-Path $plugins 'compaction.js')
opencode reload
```

Do not overwrite an existing configuration or plugin without preserving its
providers, permissions, and other settings.

OpenCode's model discovery reports the GGUF's advertised **262144-token**
context, not Ollama's configured **81920-token** runtime window. Its discovered
**32000-token** output limit also exceeds this setup's **8192-token** cap.
The explicit model limits in this file are therefore required:
**81920 context, 65536 input, and 8192 output**. Use the single named
`qwen38:latest` entry. After creating it, the source model tag can be removed
with `ollama rm SOURCE_MODEL_ID`; the weights remain referenced by `qwen38`.

Automatic compaction is enabled with a **16384-token** buffer below the input
limit, so OpenCode 2.1.2 starts compacting around **49152 tokens**, retaining up
to **8192 tokens** of recent conversation verbatim alongside its summary.
This leaves room for tool results, image-token accounting, and generation.
Provider requests allow ten minutes for cold loading and long prompt processing.
Normal chat uses medium reasoning. The built-in compaction agent uses the same
model's no-thinking variant so the 8192-token output budget is available for
the actual summary rather than hidden reasoning. This does not add another
model entry or download.
The summary-only hook replaces task-execution instructions with a concise
summary instruction targeting 1000 words and removes tools from that request.
It preserves the conversation and token limits and does not affect normal
chat. Low reasoning alone can still exhaust the output budget on long,
tool-heavy conversations.
Missing stream finish reasons remain errors rather than being accepted as
successful replies.

After applying the configuration, select **Qwen3.8 27B abliterated Q6_K**.
For an existing conversation already at the context boundary, use OpenCode's
**Compact** command once before continuing; increasing the model limit alone
does not repair the accumulated conversation.

If inference becomes unexpectedly slow after an Ollama update, check
`ollama ps`. CPU-only residency is not normal for this profile. Once the update
has finished installing its CUDA libraries, quit and restart Ollama and verify
that `ollama ps` reports **100% GPU** before resuming large-context work.

OpenCode depends on both native endpoints. Do not stop Ollama, its runner, or
the compatibility proxy to free VRAM while using that local model. Qwen at
80K uses about 25 GiB of GPU memory.

## Switch in Open WebUI or Continue

After pulling a model, refresh Open WebUI and select it from the chat's model
dropdown. Downloaded models remain on disk; Ollama swaps the loaded model when
you choose another. The next model may take longer to respond while loading.

`config.json` is a **legacy Continue JSON configuration**. If your version still
supports it, copy it to your Continue configuration directory:

```powershell
New-Item -ItemType Directory -Force "$HOME\.continue"
Copy-Item .\config.json "$HOME\.continue\config.json"
```

Back up any existing configuration before overwriting it. For current Continue
versions using `config.yaml`, add the downloaded models through Continue's
configuration UI using provider **Ollama**, API base
`http://localhost:11434`, and the exact model IDs from `config.json`.
Only select downloaded entries; having an entry in Continue does not download it.
Autocomplete uses `qwen2.5-coder:7b`, so run `.\pull.ps1 coder` if you want that.
Autocomplete and chat use the same Ollama server and may trigger model swapping.

## GitHub Copilot app

Add an **Ollama** provider with base URL `http://localhost:11434`. Edit the
selected model and save:

| Setting | Value |
|---|---|
| Wire model | `qwen38` |
| Max prompt tokens | `65536` |
| Max output tokens | `8192` |
| Reasoning effort | Medium only |

These are app settings; Continue's `config.json` does not configure Copilot.
The input and output budget leaves 8192 tokens of headroom below the 80K
server limit. It does not make arbitrarily large conversations fit.

The `ollama-compat` service repairs null or missing assistant content as an
empty string, retaining reasoning fields and tool calls. This addresses
Copilot's replay of empty assistant turns during follow-ups and compaction
([upstream issue](https://github.com/github/copilot-cli/issues/4269)).
It bounds Qwen generation and selects its supported medium reasoning level.
Other models keep their reasoning/output settings. Model discovery, native
Ollama APIs, upstream errors, and streamed responses pass through unchanged;
the service never manufactures a finish reason or successful answer.
Requests over 32 MiB are rejected. Logs contain request sizes and adaptation
counts, not conversation content.

After changing compatibility code, run `docker compose up -d --build`.
Use `docker compose logs --tail 100 ollama-compat ollama` for diagnostics.
Keep the token limits in Copilot: the compatibility service does not implement
conversation compaction itself.

System instructions and tool definitions consume part of the prompt budget
before conversation history. Compaction cannot remove those fixed costs.
Disable unneeded integrations and instructions to leave more room for conversation.
Increasing the server context alone does not change the app's prompt budget:
save the limits above in the model provider settings.

## Memory, updates, and troubleshooting

The default `OLLAMA_CONTEXT_LENGTH` is 81920 to accommodate large Copilot
requests. Change it in `.env` if needed, then run `docker compose up -d`.
Longer contexts use more VRAM; clients can override this setting, and a model's
advertised 256K context is not a guarantee it will fit at that length on a 32 GB
card.

Allow roughly 27 GB of model/runtime GPU memory for Qwen at 80K context,
plus memory for the desktop and other applications. GPU usage varies; reduce
context or unload the model when other applications need the memory.

Ollama retains a model for 30 minutes while idle, then unloads it to release
GPU memory. Requests can override retention. A cold start can take a few minutes,
depending on storage and available resources; subsequent requests use the loaded model.
The load timeout is ten minutes to tolerate slow cold starts. All services
restart automatically unless explicitly stopped.

Selecting a model in Copilot does not preload it; sending a message loads it.
Switching to a cloud model does not immediately unload the local model.
To reclaim GPU memory immediately, run
`docker compose exec -T ollama ollama stop qwen38`.
To load Qwen before chatting, run
`docker compose exec ollama ollama run qwen38`, then type `/bye`;
the model remains loaded until its idle timeout.

```powershell
docker compose exec -T ollama ollama list
docker compose exec -T ollama ollama ps
docker compose logs --tail 100 ollama
nvidia-smi
```

`ollama ps` shows whether a loaded model is entirely on the GPU. If you see CPU
offload or out-of-memory errors, reduce context or choose a smaller quant/model.
To upgrade, back up your Open WebUI data directory first (a database migration
may prevent downgrading), update the image tags in `docker-compose.yml` to the
desired stable releases, then run `docker compose pull` and
`docker compose up -d`. Pulling alone does not change pinned release versions.
If an architecture remains unsupported, choose another listed model rather than
assuming the pull succeeded means inference is supported.

To free disk space, run `docker compose exec -T ollama ollama rm MODEL_ID`.
This removes only that model, not chat history or other downloaded models.
Reduced-refusal models can produce unsafe or incorrect outputs; review outputs
and keep tool execution permissions limited.

## Checks and benchmarks

```powershell
pwsh -NoProfile -File .\tests\model-picker.tests.ps1
docker compose config --quiet
python .\tests\ollama-integration.py
python .\tests\ollama-integration.py --benchmark-context 81920
node .\tests\opencode-compaction.test.mjs
Set-Location compat
go test ./...
```

The picker check mocks Docker and does not download models.
The live integration check requires the compatibility endpoint on port 11434
and downloaded `qwen38`, using either the native setup or the Docker stack. It
checks streaming termination, repaired history, tool calls, summaries, and
continuation. It uses synthetic prompts and does not execute model-generated
tools. A Copilot app session still needs its own compaction check.

To check OpenCode Desktop itself, use its running service's loopback URL:

```powershell
$service = Get-CimInstance Win32_Process -Filter "Name = 'opencode-cli.exe'" |
    Where-Object CommandLine -Match 'serve --service'
$listener = Get-NetTCPConnection -State Listen -OwningProcess $service.ProcessId |
    Where-Object LocalAddress -EQ '127.0.0.1'
python .\tests\opencode-integration.py --url "http://127.0.0.1:$($listener.LocalPort)"
```

This check uses the existing local service credentials without logging them.
It creates a separate verification session, reads `compat/go.mod` with a real
OpenCode tool, and supplies synthetic context to verify automatic compaction
and marker-preserving continuation. It does not continue the user's task and
instructs the agent not to run shell commands. It leaves the verification session
available for inspection in OpenCode.

The optional context benchmark fills most of the selected window, checks
marker retrieval and GPU residency, and requires at least 2 GiB GPU headroom.
It changes the loaded model's context; run the normal integration check
afterward to return to the server default.
