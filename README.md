# Local models with Ollama and GitHub Copilot

Download only the models you want, then select them in GitHub Copilot,
Open WebUI, or Continue in VS Code. The defaults target a single RTX 5090 with 32 GB VRAM:
one loaded model, one concurrent request, and an 80K context window.

The stack pins container versions:
[Ollama 0.40.1](https://github.com/ollama/ollama/releases/tag/v0.40.1) and
[Open WebUI v0.11.4](https://github.com/open-webui/open-webui/releases/tag/v0.11.4).
They do not follow floating `latest` or `main` tags.

## Start the stack

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
The compatibility service uses **medium** for Qwen Chat Completions requests
and caps generation at 8192 tokens, including reasoning. The Ollama container
uses the CUDA 12 backend for GPU inference. Verify `ollama ps` reports
`100% GPU` after a request.

On Bash/Linux/macOS, pass model IDs explicitly (any Ollama-compatible ID works):

```bash
bash pull.sh 'hf.co/huihui-ai/Huihui-Qwen3.8-27B-abliterated-GGUF:Huihui-Qwen3.8-27B-abliterated-Q6_K.gguf'
bash pull.sh qwen2.5-coder:7b deepseek-r1:14b
```

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
Set-Location compat
go test ./...
```

The picker check mocks Docker and does not download models.
The live integration check requires the stack and downloaded `qwen38`; it
checks streaming termination, repaired history, tool calls, summaries, and
continuation. It uses synthetic prompts and does not execute model-generated
tools. A Copilot app session still needs its own compaction check.
The optional context benchmark fills most of the selected window, checks
marker retrieval and GPU residency, and requires at least 2 GiB GPU headroom.
It changes the loaded model's context; run the normal integration check
afterward to return to the server default.
