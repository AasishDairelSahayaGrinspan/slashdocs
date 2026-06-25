import { useTitle } from "@/lib/use-title";
import { Bot, ExternalLink, KeyRound, Rocket } from "lucide-react";
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
    <div style={{ maxWidth: "896px", margin: "0 auto", paddingLeft: "24px", paddingRight: "24px", paddingTop: "40px", paddingBottom: "40px" }}>
      <div style={{ marginBottom: "24px" }}>
        <h1 style={{ fontSize: "24px", fontWeight: 600, letterSpacing: "-0.02em" }}>Developers</h1>
        <p style={{ marginTop: "4px", fontSize: "14px", color: "#71717a" }}>
          API reference and authentication model for building against aidocs directly.
        </p>
      </div>

      <div style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
        <Card>
          <CardHeader>
            <CardTitle style={{ display: "flex", alignItems: "center", gap: "8px" }}>
              <Rocket style={{ height: "16px", width: "16px" }} /> Looking to set up your agent?
            </CardTitle>
            <CardDescription>
              The CLI install and agent skill setup live in the setup guide, not here.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button asChild>
              <Link to="/app/start">
                Open setup guide <ExternalLink style={{ height: "14px", width: "14px" }} />
              </Link>
            </Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle style={{ display: "flex", alignItems: "center", gap: "8px" }}>
              <KeyRound style={{ height: "16px", width: "16px" }} /> Authentication model
            </CardTitle>
            <CardDescription>Two ways to authenticate against the API.</CardDescription>
          </CardHeader>
          <CardContent style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
            <div>
              <div style={{ fontSize: "14px", fontWeight: 500 }}>User session</div>
              <p style={{ marginTop: "4px", fontSize: "12px", color: "#71717a" }}>
                Google OAuth via the browser. The CLI uses this when you run{" "}
                <code style={{ fontFamily: "ui-monospace, monospace" }}>aidocs auth login</code>. Best for
                interactive use and any agent running locally on your machine.
              </p>
            </div>
            <div>
              <div style={{ fontSize: "14px", fontWeight: 500 }}>Service account token</div>
              <p style={{ marginTop: "4px", fontSize: "12px", color: "#71717a" }}>
                Bearer token for headless agents that can't run OAuth — n8n, scheduled
                jobs, CI. Mint a key under{" "}
                <Link
                  style={{ fontWeight: 500, color: "#18181b" }}
                  to="/app/settings/service-accounts"
                >
                  Service accounts
                </Link>{" "}
                and pass it as{" "}
                <code style={{ fontFamily: "ui-monospace, monospace" }}>Authorization: Bearer …</code>.
              </p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle style={{ display: "flex", alignItems: "center", gap: "8px" }}>
              <Bot style={{ height: "16px", width: "16px" }} /> Service accounts
            </CardTitle>
            <CardDescription>
              Headless identities for agents that can't run OAuth (n8n, CI, scheduled
              jobs). Mint bearer tokens here. Most users should ignore this — the CLI
              handles auth automatically when an agent runs it on your machine.
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
          <CardContent style={{ display: "flex", gap: "8px" }}>
            <Button asChild variant="outline">
              <a href="/api-docs" target="_blank" rel="noreferrer">
                <ExternalLink style={{ height: "16px", width: "16px" }} /> Swagger UI
              </a>
            </Button>
            <Button asChild variant="outline">
              <a href="/openapi.json" target="_blank" rel="noreferrer">
                <ExternalLink style={{ height: "16px", width: "16px" }} /> openapi.json
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
