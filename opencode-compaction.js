export default {
    id: "skynet.compaction",
    async setup(ctx) {
        await ctx.session.hook("compaction", (request) => {
            if (request.model.id !== "qwen38:latest") return;
            request.system = [{
                type: "text",
                text: "Write a concise continuation summary, not a task response. Follow the requested headings. Use at most 1000 words. Preserve unresolved requirements, decisions, exact identifiers, changed files, blockers, and the next action. Omit transcripts, logs, repeated plans, and details recoverable from files. Do not call tools or continue the task.",
            }];
            request.tools = {};
        }, { providerID: "ollama" });
    },
};
