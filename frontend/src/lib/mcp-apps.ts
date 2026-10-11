import type { Brand } from "@/components/brand-mark";

const NAME = "slashdocs";

// One link per agent app that adds the slashdocs remote MCP server. Each app
// discovers the OAuth metadata from the URL, so no key changes hands.
export function mcpApps(url: string): { id: Brand; label: string; href: string }[] {
  // Claude Code and Codex have no install link, but each opens a new session
  // with a typed prompt. Nothing runs until the user sends it and allows it.
  const ask = (command: string) =>
    encodeURIComponent(`Add the ${NAME} MCP server. Run this command:\n${command}`);
  return [
    {
      // Opens the add-connector dialog with the fields filled in. The
      // connector then syncs to the Claude desktop app and to Claude Code.
      id: "claude",
      label: "Claude",
      href: `https://claude.ai/settings/connectors?${new URLSearchParams({
        modal: "add-custom-connector",
        mcpName: NAME,
        mcpServerUrl: url,
      })}`,
    },
    {
      id: "claude-code",
      label: "Claude Code",
      href: `claude-cli://open?q=${ask(
        `claude mcp add --transport http --scope user ${NAME} ${url} && claude mcp login ${NAME}`,
      )}`,
    },
    {
      // The ChatGPT desktop app, the Codex CLI, and the Codex IDE extension
      // share this setup.
      id: "codex",
      label: "ChatGPT / Codex",
      href: `codex://new?prompt=${ask(`codex mcp add ${NAME} --url ${url}`)}`,
    },
    {
      id: "cursor",
      label: "Cursor",
      href: `cursor://anysphere.cursor-deeplink/mcp/install?name=${NAME}&config=${btoa(JSON.stringify({ url }))}`,
    },
    {
      id: "vscode",
      label: "VS Code",
      href: `vscode:mcp/install?${encodeURIComponent(JSON.stringify({ name: NAME, type: "http", url }))}`,
    },
  ];
}
