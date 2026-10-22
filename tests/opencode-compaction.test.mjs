import assert from "node:assert/strict";
import plugin from "../opencode-compaction.js";

let compact;
await plugin.setup({
    session: {
        async hook(name, callback, scope) {
            assert.equal(name, "compaction");
            assert.equal(scope.providerID, "ollama");
            compact = callback;
        },
    },
});
const messages = [{ role: "user", content: "Keep the task marker." }];
const request = {
    model: { id: "qwen38:latest" },
    system: [{ type: "text", text: "Continue the task." }],
    tools: { shell: {} },
    messages,
    options: { maxTokens: 8192 },
};
compact(request);
assert.match(request.system[0].text, /at most 1000 words/);
assert.deepEqual(request.tools, {});
assert.equal(request.messages, messages);
assert.equal(request.options.maxTokens, 8192);
const other = { model: { id: "other" }, system: [], tools: { shell: {} } };
compact(other);
assert.deepEqual(other.tools, { shell: {} });
