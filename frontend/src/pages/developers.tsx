import { useTitle } from "@/lib/use-title";
import { Bot, Cable, ExternalLink, KeyRound, Rocket } from "lucide-react";
import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { CodeBlock } from "@/components/ui/misc";
import { publicURL } from "@/lib/config";

export function DevelopersPage() {
  useTitle("Developers");
  const url = publicURL();
  return (
    <div className="mx-auto max-w-4xl px-4 py-6 sm:px-6 sm:py-10">
      <div className="mb-6">
        <h1 className="text-balance text-2xl font-semibold tracking-tight">
          Developers
        </h1>
        <p className="mt-1 text-pretty text-sm text-[var(--color-fg-muted)]">
          Connect an agent over MCP, use slashdocs from a terminal, or build directly
          against the API.
        </p>
      </div>

      <div className="space-y-4">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Cable className="h-4 w-4" /> Connect an MCP client
            </CardTitle>
            <CardDescription>
              Connect directly to this slashdocs server. There is no CLI, local bridge,
              or separate MCP process to install.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <CodeBlock>{`${url}/mcp`}</CodeBlock>
            <p className="text-pretty text-xs text-[var(--color-fg-muted)]">
              Paste this URL into a remote MCP client. Interactive clients open the
              normal slashdocs sign-in flow automatically; unattended agents can use a
              service-account token.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Rocket className="h-4 w-4" /> Set up the CLI
            </CardTitle>
            <CardDescription>
              Prefer working from a terminal or letting a local agent run slashdocs
              commands? Install the CLI and bundled agent skill.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button asChild>
              <Link to="/app/start">
                Open CLI setup guide <ExternalLink className="h-3.5 w-3.5" />
              </Link>
            </Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <KeyRound className="h-4 w-4" /> Authentication model
            </CardTitle>
            <CardDescription>
              Interactive sign-in for people, bearer tokens for unattended agents.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <div className="text-sm font-medium">Interactive sign-in</div>
              <p className="mt-1 text-pretty text-xs text-[var(--color-fg-muted)]">
                OpenID Connect through this server's configured identity provider.
                Remote MCP clients open the sign-in flow automatically; the CLI uses it
                when you run <code className="font-mono">slashdocs auth login</code>.
              </p>
            </div>
            <div>
              <div className="text-sm font-medium">Service account token</div>
              <p className="mt-1 text-pretty text-xs text-[var(--color-fg-muted)]">
                Bearer token for headless agents that can't run OAuth — n8n, scheduled
                jobs, CI. Mint a key under{" "}
                <Link
                  className="font-medium text-[var(--color-fg)] underline-offset-2 hover:underline"
                  to="/app/settings/service-accounts"
                >
                  Service accounts
                </Link>{" "}
                and pass it as{" "}
                <code className="font-mono">Authorization: Bearer …</code>.
              </p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Bot className="h-4 w-4" /> Service accounts
            </CardTitle>
            <CardDescription>
              Headless identities for agents that can’t run OAuth (n8n, CI, scheduled
              jobs). Mint bearer tokens here. Most users should ignore this — remote MCP
              clients and the CLI both handle interactive sign-in automatically.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button asChild variant="outline">
              <Link to="/app/settings/service-accounts">Manage service accounts</Link>
            </Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>API reference</CardTitle>
            <CardDescription>OpenAPI spec and Swagger UI.</CardDescription>
          </CardHeader>
          <CardContent className="flex gap-2">
            <Button asChild variant="outline">
              <a href="/api-docs" target="_blank" rel="noreferrer">
                <ExternalLink className="h-4 w-4" /> Swagger UI
              </a>
            </Button>
            <Button asChild variant="outline">
              <a href="/openapi.json" target="_blank" rel="noreferrer">
                <ExternalLink className="h-4 w-4" /> openapi.json
              </a>
            </Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Example: raw HTTP</CardTitle>
            <CardDescription>
              List documents using a service-account bearer token.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <CodeBlock>{`curl -H "Authorization: Bearer <YOUR_KEY>" \\
  ${url}/v1/documents`}</CodeBlock>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
