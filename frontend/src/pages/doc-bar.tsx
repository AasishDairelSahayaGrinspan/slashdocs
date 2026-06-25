import { Link, useNavigate } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "react-hot-toast";
import { errorMessage } from "@/lib/errors";
import {
  ArrowLeft,
  Check,
  ExternalLink,
  FolderIcon,
  GitBranch,
  MoreHorizontal,
  PanelRightClose,
  PanelRightOpen,
  Share2,
  Trash2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dropdown,
  DropdownContent,
  DropdownItem,
  DropdownSeparator,
  DropdownTrigger,
} from "@/components/ui/dropdown";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Sheet, SheetTrigger } from "@/components/ui/sheet";
import { DeleteDocumentDialog } from "@/components/delete-document-dialog";
import { api, type Principal, type Version } from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { ShareSheet } from "./doc-share-sheet";
import { UploadVersionItem } from "./doc-upload-version";

export function DocBar({
  title,
  docId,
  owner,
  principal,
  parentFolder,
  current,
  latest,
  versions,
  onSelectVersion,
  commentsOpen,
  onToggleComments,
}: {
  title: string;
  docId: string;
  owner?: Principal;
  principal?: Principal;
  parentFolder?: { id: string; title: string };
  current: string;
  latest: string;
  versions: Version[];
  onSelectVersion: (versionID: string) => void;
  commentsOpen: boolean;
  onToggleComments: () => void;
}) {
  const nav = useNavigate();
  const q = useQueryClient();
  const currentVersion = versions.find((v) => v.id === current);
  const isViewingOlder = !!current && !!latest && current !== latest;
  const canManage = samePrincipal(principal, owner);
  const del = useMutation({
    mutationFn: () => api.deleteDocument(docId),
    onSuccess: () => {
      toast.success("Document deleted.");
      q.invalidateQueries({ queryKey: queryKeys.documents() });
      nav("/app/documents");
    },
    onError: (e) => toast.error(errorMessage(e, "Delete failed")),
  });
  return (
    <div style={{
      display: "flex",
      height: "56px",
      alignItems: "center",
      gap: "12px",
      borderBottom: "1px solid #e4e4e7",
      background: "#fff",
      padding: "0 16px",
    }}>
      <Button asChild variant="ghost" size="icon">
        <Link to="/app/documents" aria-label="Back to documents">
          <ArrowLeft style={{ height: "16px", width: "16px" }} />
        </Link>
      </Button>
      <div style={{ minWidth: 0, flex: 1 }}>
        {parentFolder ? (
          <div style={{ display: "flex", alignItems: "center", gap: "4px", fontSize: "12px", color: "#71717a" }}>
            <FolderIcon style={{ height: "12px", width: "12px", flexShrink: 0 }} />
            <Link to={`/app/folders/${parentFolder.id}`} style={{ color: "#71717a" }}>{parentFolder.title}</Link>
          </div>
        ) : null}
        <div style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontSize: "14px", fontWeight: 600 }}>{title}</div>
        {isViewingOlder && currentVersion ? (
          <div style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontSize: "11px", color: "#d97706" }}>
            {`Viewing older version - version ${currentVersion.number}`}
          </div>
        ) : null}
      </div>
      {isViewingOlder ? (
        <Button variant="secondary" size="sm" onClick={() => onSelectVersion(latest)}>
          Back to latest
        </Button>
      ) : null}
      <VersionHistoryDialog
        current={current}
        latest={latest}
        versions={versions}
        onSelectVersion={onSelectVersion}
      />
      {canManage ? (
        <Sheet>
          <SheetTrigger asChild>
            <Button variant="outline" size="sm">
              <Share2 style={{ height: "16px", width: "16px" }} /> Share
            </Button>
          </SheetTrigger>
          <ShareSheet docId={docId} />
        </Sheet>
      ) : null}
      <Button
        variant="ghost"
        size="icon"
        onClick={onToggleComments}
        aria-label={commentsOpen ? "Hide comments" : "Show comments"}
      >
        {commentsOpen ? (
          <PanelRightClose style={{ height: "16px", width: "16px" }} />
        ) : (
          <PanelRightOpen style={{ height: "16px", width: "16px" }} />
        )}
      </Button>
      <Dropdown>
        <DropdownTrigger asChild>
          <Button variant="ghost" size="icon" aria-label="More">
            <MoreHorizontal style={{ height: "16px", width: "16px" }} />
          </Button>
        </DropdownTrigger>
        <DropdownContent>
          <DropdownItem
            onSelect={() =>
              window.open(
                `/v1/documents/${docId}/versions/${current}/html`,
                "_blank",
                "noopener,noreferrer",
              )
            }
          >
            <ExternalLink style={{ height: "16px", width: "16px" }} /> Open raw HTML
          </DropdownItem>
          {canManage ? (
            <>
              <UploadVersionItem docId={docId} baseVersion={latest || current} />
              <DropdownSeparator />
              <DeleteDocumentDialog
                title={title}
                onConfirm={() => del.mutate()}
                trigger={
                  <DropdownItem
                    onSelect={(e) => e.preventDefault()}
                    style={{ color: "#dc2626" }}
                  >
                    <Trash2 style={{ height: "16px", width: "16px" }} /> Delete document
                  </DropdownItem>
                }
              />
            </>
          ) : null}
        </DropdownContent>
      </Dropdown>
    </div>
  );
}

function samePrincipal(a?: Principal, b?: Principal) {
  return !!a && !!b && a.type === b.type && a.id === b.id;
}

function VersionHistoryDialog({
  current,
  latest,
  versions,
  onSelectVersion,
}: {
  current: string;
  latest: string;
  versions: Version[];
  onSelectVersion: (versionID: string) => void;
}) {
  const sorted = [...versions].sort((a, b) => b.number - a.number);
  const currentVersion = versions.find((v) => v.id === current);
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm">
          <GitBranch style={{ height: "16px", width: "16px" }} />
          {currentVersion ? `v${currentVersion.number}` : "Versions"}
        </Button>
      </DialogTrigger>
      <DialogContent style={{ maxWidth: "672px" }}>
        <DialogHeader>
          <DialogTitle>Version history</DialogTitle>
          <DialogDescription>
            Immutable HTML revisions for this document. Open any version to inspect the
            exact HTML your reviewers saw.
          </DialogDescription>
        </DialogHeader>
        {sorted.length === 0 ? (
          <div style={{ borderRadius: "8px", border: "1px solid #e4e4e7", padding: "16px", fontSize: "14px", color: "#71717a" }}>
            No versions available yet.
          </div>
        ) : (
          <div style={{ maxHeight: "60vh", display: "flex", flexDirection: "column", gap: "8px", overflow: "auto", paddingRight: "4px" }}>
            {sorted.map((v) => (
              <button
                key={v.id}
                type="button"
                onClick={() => onSelectVersion(v.id)}
                style={{
                  display: "flex",
                  width: "100%",
                  gap: "12px",
                  borderRadius: "12px",
                  border: "1px solid #e4e4e7",
                  padding: "16px",
                  textAlign: "left",
                  transition: "background-color 0.15s",
                  background: v.id === current ? "rgba(238,240,255,0.35)" : "#fff",
                  cursor: "pointer",
                }}
                aria-current={v.id === current ? "true" : undefined}
              >
                <div style={{
                  display: "flex",
                  height: "32px",
                  width: "32px",
                  flexShrink: 0,
                  alignItems: "center",
                  justifyContent: "center",
                  borderRadius: "9999px",
                  background: "#eef0ff",
                  fontSize: "14px",
                  fontWeight: 600,
                  color: "#2028ff",
                }}>
                  {v.number}
                </div>
                <div style={{ minWidth: 0, flex: 1 }}>
                  <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: "8px" }}>
                    <div style={{ fontWeight: 500 }}>Version {v.number}</div>
                    {v.id === current ? (
                      <Badge variant="accent">
                        <Check style={{ height: "12px", width: "12px" }} /> Viewing
                      </Badge>
                    ) : null}
                    {v.id === latest ? <Badge variant="muted">Latest</Badge> : null}
                  </div>
                  <div style={{ marginTop: "4px", fontSize: "14px", color: "#71717a" }}>
                    {v.change_summary ||
                      (v.number === 1 ? "Initial version" : "No change summary")}
                  </div>
                  <div style={{ marginTop: "8px", display: "flex", flexWrap: "wrap", gap: "16px", fontSize: "12px", color: "#71717a" }}>
                    <span>Created by {principalLabel(v.created_by)}</span>
                    <span style={{ fontFamily: "ui-monospace, monospace" }}>{v.id}</span>
                  </div>
                </div>
              </button>
            ))}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

function principalLabel(p?: Principal) {
  if (!p) return "Unknown";
  if (p.name) return p.name;
  if (p.email) return p.email;
  if (p.type === "service_account") return p.id || "Service account";
  return p.id || "Unknown";
}
