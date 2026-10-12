import { describe, expect, it } from "vitest";
import { mcpApps } from "./mcp-apps";

const url = "https://docs.example.com/mcp";
const link = (label: string) =>
  new URL(mcpApps(url).find((a) => a.label === label)!.href);

describe("mcpApps", () => {
  it("fills in the Claude add-connector dialog", () => {
    const l = link("Claude");
    expect(l.origin + l.pathname).toBe("https://claude.ai/customize/connectors");
    expect(Object.fromEntries(l.searchParams)).toEqual({
      modal: "add-custom-connector",
      connectorName: "slashdocs",
      connectorUrl: url,
    });
  });

  it("types the setup commands into Claude Code and Codex", () => {
    expect(link("Claude Code").searchParams.get("q")).toContain(
      `claude mcp add --transport http --scope user slashdocs ${url} && claude mcp login slashdocs`,
    );
    expect(link("ChatGPT / Codex").searchParams.get("prompt")).toContain(
      `codex mcp add slashdocs --url ${url}`,
    );
  });

  it("encodes the Cursor install link as base64 config", () => {
    const l = link("Cursor");
    expect(l.protocol).toBe("cursor:");
    expect(l.searchParams.get("name")).toBe("slashdocs");
    expect(JSON.parse(atob(l.searchParams.get("config")!))).toEqual({ url });
  });

  it("encodes the VS Code install link as URL-encoded JSON", () => {
    const [scheme, query] = link("VS Code").href.split("?");
    expect(scheme).toBe("vscode:mcp/install");
    expect(JSON.parse(decodeURIComponent(query))).toEqual({
      name: "slashdocs",
      type: "http",
      url,
    });
  });
});
