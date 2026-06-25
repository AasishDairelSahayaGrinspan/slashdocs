import { useTitle } from "@/lib/use-title";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "react-hot-toast";
import { errorMessage } from "@/lib/errors";
import { Bot, KeyRound, Plus, Power, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { CodeBlock, EmptyState, Skeleton } from "@/components/ui/misc";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { api, type ServiceAccount } from "@/api";
import { publicURL } from "@/lib/config";
import { queryKeys } from "@/lib/queryKeys";
import { BOT_DOMAIN_SUFFIX } from "@/lib/constants";

export function ServiceAccountsPage() {
  useTitle("Service accounts");
  const q = useQueryClient();
  const sas = useQuery({
    queryKey: queryKeys.serviceAccounts(),
    queryFn: api.listServiceAccounts,
  });
  const [creating, setCreating] = useState(false);
  const items = sas.data?.items || [];
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const selected = items.find((s) => s.id === selectedId) || items[0] || null;

  return (
    <div style={{ maxWidth: "1152px", margin: "0 auto", paddingLeft: "24px", paddingRight: "24px", paddingTop: "40px", paddingBottom: "40px" }}>
      <div style={{ marginBottom: "24px", display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: "16px" }}>
        <div>
          <h1 style={{ fontSize: "24px", fontWeight: 600, letterSpacing: "-0.02em" }}>Bots</h1>
          <p style={{ marginTop: "4px", maxWidth: "672px", fontSize: "14px", color: "#71717a" }}>
            Bots are their own identities — share docs with them just like you would
            with a person. Use them for n8n workflows, scheduled jobs, or anything that
            runs without you.
          </p>
        </div>
        <Button onClick={() => setCreating(true)}>
          <Plus style={{ height: "16px", width: "16px" }} /> New bot
        </Button>
      </div>
      <NewBotDialog
        open={creating}
        onOpenChange={setCreating}
        onCreated={(sa) => {
          q.invalidateQueries({ queryKey: queryKeys.serviceAccounts() });
          setSelectedId(sa.id);
        }}
      />

      <div style={{ display: "grid", gap: "24px", gridTemplateColumns: "280px 1fr" }}>
        <div style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
          {sas.isLoading ? (
            <>
              <Skeleton style={{ height: "48px", width: "100%" }} />
              <Skeleton style={{ height: "48px", width: "100%" }} />
            </>
          ) : sas.error ? (
            <div style={{ borderRadius: "12px", border: "1px dashed #e4e4e7", padding: "24px", textAlign: "center", fontSize: "14px", color: "#dc2626" }}>
              Could not load bots.
            </div>
          ) : items.length === 0 ? (
            <div style={{ borderRadius: "12px", border: "1px dashed #e4e4e7", padding: "24px", textAlign: "center", fontSize: "14px", color: "#71717a" }}>
              No bots yet. Create one to share docs with it.
            </div>
          ) : (
            <ul style={{ display: "flex", flexDirection: "column", gap: "4px", listStyle: "none", padding: 0, margin: 0 }}>
              {items.map((sa) => {
                const id = sa.id;
                const active = selected?.id === id;
                return (
                  <li key={id}>
                    <button
                      onClick={() => setSelectedId(id)}
                      style={{
                        display: "flex",
                        width: "100%",
                        alignItems: "center",
                        gap: "8px",
                        borderRadius: "6px",
                        border: `1px solid ${active ? "#2028ff" : "#e4e4e7"}`,
                        background: active ? "#eef0ff" : "#fff",
                        padding: "8px 12px",
                        textAlign: "left",
                        fontSize: "14px",
                        transition: "background-color 0.15s",
                        cursor: "pointer",
                      }}
                    >
                      <Bot style={{ height: "16px", width: "16px", flexShrink: 0, color: "#71717a" }} />
                      <div style={{ minWidth: 0, flex: 1 }}>
                        <div style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontWeight: 500 }}>{sa.name}</div>
                        <div style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontFamily: "ui-monospace, monospace", fontSize: "10px", color: "#71717a" }}>
                          {id}
                        </div>
                      </div>
                      {sa.disabled && <Badge variant="warning">disabled</Badge>}
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
        </div>

        <div>
          {selected ? (
            <ServiceAccountDetail sa={selected} />
          ) : (
            <EmptyState
              icon={<Bot style={{ height: "20px", width: "20px" }} />}
              title="No bot selected"
              description="Pick one on the left to manage its keys."
            />
          )}
        </div>
      </div>
    </div>
  );
}

function ServiceAccountDetail({ sa }: { sa: ServiceAccount }) {
  const { id, name, disabled = false } = sa;
  const q = useQueryClient();
  const url = publicURL();
  const keys = useQuery({
    queryKey: queryKeys.serviceAccountKeys(id),
    queryFn: () => api.listServiceAccountKeys(id),
  });
  const [revealToken, setRevealToken] = useState<{
    token: string;
    keyName: string;
  } | null>(null);
  const [keyName, setKeyName] = useState("default");
  const createKey = useMutation({
    mutationFn: () => api.createServiceAccountKey(id, keyName),
    onSuccess: (r) => {
      setRevealToken({ token: r.token, keyName });
      setKeyName("default");
      q.invalidateQueries({ queryKey: queryKeys.serviceAccountKeys(id) });
    },
    onError: (e) => toast.error(errorMessage(e, "Failed")),
  });
  const revokeKey = useMutation({
    mutationFn: (keyID: string) => api.revokeServiceAccountKey(id, keyID),
    onSuccess: () => {
      toast.success("Key revoked.");
      q.invalidateQueries({ queryKey: queryKeys.serviceAccountKeys(id) });
    },
    onError: (e) => toast.error(errorMessage(e, "Could not revoke key")),
  });
  const toggle = useMutation({
    mutationFn: () => api.updateServiceAccount(id, name, !disabled),
    onSuccess: () => {
      toast.success(disabled ? "Enabled." : "Disabled.");
      q.invalidateQueries({ queryKey: queryKeys.serviceAccounts() });
    },
    onError: (e) => toast.error(errorMessage(e, "Could not update bot")),
  });

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
      <Card>
        <CardHeader style={{ display: "flex", flexDirection: "row", alignItems: "flex-start", justifyContent: "space-between" }}>
          <div>
            <CardTitle style={{ display: "flex", alignItems: "center", gap: "8px" }}>
              <Bot style={{ height: "16px", width: "16px" }} /> {name}
            </CardTitle>
            <div style={{ marginTop: "4px", fontFamily: "ui-monospace, monospace", fontSize: "12px", color: "#71717a" }}>
              {id}
            </div>
          </div>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button variant="outline" size="sm">
                <Power style={{ height: "14px", width: "14px" }} />
                {disabled ? "Enable" : "Disable"}
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogTitle>
                {disabled ? "Enable" : "Disable"} "{name}"?
              </AlertDialogTitle>
              <AlertDialogDescription>
                {disabled
                  ? "Agents using active keys will be able to authenticate again."
                  : "All agents using this service account will immediately lose access. Keys and grants are retained."}
              </AlertDialogDescription>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={() => toggle.mutate()}>
                  {disabled ? "Enable" : "Disable"}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </CardHeader>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle style={{ display: "flex", alignItems: "center", gap: "8px", fontSize: "14px" }}>
            <KeyRound style={{ height: "16px", width: "16px" }} /> API keys
          </CardTitle>
        </CardHeader>
        <CardContent style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
          <form
            style={{ display: "flex", gap: "8px" }}
            onSubmit={(e) => {
              e.preventDefault();
              createKey.mutate();
            }}
          >
            <Input
              value={keyName}
              onChange={(e) => setKeyName(e.target.value)}
              placeholder="Key name"
            />
            <Button type="submit" disabled={!keyName || createKey.isPending}>
              <Plus style={{ height: "16px", width: "16px" }} /> New key
            </Button>
          </form>
          <div style={{ display: "flex", flexDirection: "column", gap: "4px" }}>
            {keys.isLoading ? (
              <>
                <Skeleton style={{ height: "40px", width: "100%" }} />
                <Skeleton style={{ height: "40px", width: "100%" }} />
              </>
            ) : keys.error ? (
              <div style={{ borderRadius: "6px", border: "1px dashed #e4e4e7", padding: "16px", textAlign: "center", fontSize: "12px", color: "#dc2626" }}>
                Could not load keys.
              </div>
            ) : keys.data?.items?.length ? (
              keys.data.items.map((k) => (
                <div
                  key={k.id}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: "12px",
                    borderRadius: "6px",
                    border: "1px solid #e4e4e7",
                    background: "#fff",
                    padding: "8px 12px",
                    fontSize: "14px",
                  }}
                >
                  <KeyRound style={{ height: "14px", width: "14px", color: "#71717a" }} />
                  <span style={{ fontWeight: 500 }}>{k.name}</span>
                  <span style={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontFamily: "ui-monospace, monospace", fontSize: "12px", color: "#71717a" }}>
                    {k.id}
                  </span>
                  <AlertDialog>
                    <AlertDialogTrigger asChild>
                      <Button variant="ghost" size="icon" aria-label="Revoke">
                        <Trash2 style={{ height: "14px", width: "14px", color: "#dc2626" }} />
                      </Button>
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                      <AlertDialogTitle>Revoke key "{k.name}"?</AlertDialogTitle>
                      <AlertDialogDescription>
                        Agents using this key will immediately lose access.
                      </AlertDialogDescription>
                      <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                          variant="danger"
                          onClick={() => revokeKey.mutate(k.id)}
                        >
                          Revoke
                        </AlertDialogAction>
                      </AlertDialogFooter>
                    </AlertDialogContent>
                  </AlertDialog>
                </div>
              ))
            ) : (
              <div style={{ borderRadius: "6px", border: "1px dashed #e4e4e7", padding: "16px", textAlign: "center", fontSize: "12px", color: "#71717a" }}>
                No keys yet. Mint one to start using the CLI.
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle style={{ fontSize: "14px" }}>Headless agent quickstart</CardTitle>
        </CardHeader>
        <CardContent>
          <p style={{ marginBottom: "8px", fontSize: "12px", color: "#71717a" }}>
            Use the minted key as a bearer token from any HTTP client, or with the CLI
            in non-interactive mode:
          </p>
          <CodeBlock>{`aidocs auth login --token <YOUR_KEY>
aidocs docs create report.html`}</CodeBlock>
          <p style={{ marginTop: "12px", fontSize: "12px", color: "#71717a" }}>For raw HTTP:</p>
          <CodeBlock>{`curl -H "Authorization: Bearer <YOUR_KEY>" \\
  ${url}/v1/documents`}</CodeBlock>
        </CardContent>
      </Card>

      <Dialog open={!!revealToken} onOpenChange={(o) => !o && setRevealToken(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Copy this key now</DialogTitle>
            <DialogDescription>
              This is the only time the secret token will be shown. Store it somewhere
              safe.
            </DialogDescription>
          </DialogHeader>
          {revealToken && (
            <>
              <div style={{ marginBottom: "4px", fontSize: "12px", fontWeight: 500, color: "#71717a" }}>
                {revealToken.keyName}
              </div>
              <CodeBlock>{revealToken.token}</CodeBlock>
              <div style={{ marginTop: "16px", fontSize: "12px", color: "#71717a" }}>
                Use it as a bearer token from your headless agent, or:
              </div>
              <CodeBlock>{`aidocs auth login ${url} --token ${revealToken.token}`}</CodeBlock>
            </>
          )}
          <DialogFooter>
            <Button onClick={() => setRevealToken(null)}>Done</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function NewBotDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onCreated: (sa: { id: string; name: string }) => void;
}) {
  const [value, setValue] = useState("");
  const [result, setResult] = useState<{ name: string; token: string } | null>(null);
  const trimmed = value.trim();
  const at = trimmed.indexOf("@");
  const label = at >= 0 ? trimmed.slice(0, at) : trimmed;
  const domain = at >= 0 ? trimmed.slice(at + 1) : "";
  const hasDomain = at >= 0;
  const domainLooksValid = !hasDomain || domain.endsWith(BOT_DOMAIN_SUFFIX);
  const create = useMutation({
    mutationFn: () => api.createServiceAccount(label, hasDomain ? domain : undefined),
    onSuccess: (r) => {
      setResult({ name: r.name, token: r.key.token });
      onCreated({ id: r.id, name: r.name });
    },
    onError: (e) => toast.error(errorMessage(e, "Failed")),
  });
  function reset() {
    setValue("");
    setResult(null);
  }
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o);
        if (!o) reset();
      }}
    >
      <DialogContent>
        {!result ? (
          <>
            <DialogHeader>
              <DialogTitle>New bot</DialogTitle>
              <DialogDescription>
                Bots are their own identities — share docs with them just like you would
                with a person.
              </DialogDescription>
            </DialogHeader>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                if (label) create.mutate();
              }}
              style={{ display: "flex", flexDirection: "column", gap: "16px" }}
            >
              <div>
                <label style={{ marginBottom: "4px", display: "block", fontSize: "12px", fontWeight: 500, color: "#71717a" }}>
                  Name
                </label>
                <Input
                  placeholder="n8n-prod   or   n8n-prod@ops.team.bot"
                  value={value}
                  onChange={(e) => setValue(e.target.value)}
                  autoCapitalize="off"
                  autoCorrect="off"
                  spellCheck={false}
                  autoFocus
                />
                <p style={{ marginTop: "4px", fontSize: "11px", color: "#71717a" }}>
                  Letters, numbers, and hyphens. Add{" "}
                  <code style={{ fontFamily: "ui-monospace, monospace" }}>@something.bot</code> to pick your bot's
                  address, or skip it and we'll pick one for you.
                </p>
                {hasDomain && !domainLooksValid && (
                  <p style={{ marginTop: "4px", fontSize: "11px", color: "#dc2626" }}>
                    Addresses must end in {BOT_DOMAIN_SUFFIX}.
                  </p>
                )}
              </div>

              <DialogFooter>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => onOpenChange(false)}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  disabled={!label || !domainLooksValid || create.isPending}
                >
                  Create bot
                </Button>
              </DialogFooter>
            </form>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>Meet {result.name}</DialogTitle>
              <DialogDescription>
                Share docs with this address to let your bot read or edit them.
              </DialogDescription>
            </DialogHeader>
            <CodeBlock>{result.name}</CodeBlock>
            <div style={{ marginTop: "20px", borderRadius: "10px", border: "1px solid rgba(217,119,6,0.4)", background: "rgba(217,119,6,0.1)", padding: "12px" }}>
              <div style={{ marginBottom: "4px", fontSize: "12px", fontWeight: 500, textTransform: "uppercase", letterSpacing: "0.05em", color: "#71717a" }}>
                Bot key
              </div>
              <div style={{ marginBottom: "8px", fontSize: "14px", fontWeight: 600, color: "#d97706" }}>
                Copy this key now — you won't see it again.
              </div>
              <CodeBlock>{result.token}</CodeBlock>
            </div>
            <DialogFooter>
              <Button onClick={() => onOpenChange(false)}>Done</Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
