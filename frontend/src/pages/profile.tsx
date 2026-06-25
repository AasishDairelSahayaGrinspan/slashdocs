import { useNavigate } from "react-router-dom";
import { useTitle } from "@/lib/use-title";
import { LogOut, Monitor, Moon, Sun } from "lucide-react";
import { Avatar, Center, Skeleton } from "@/components/ui/misc";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useMe } from "@/lib/hooks";
import { useTheme } from "@/lib/theme";
import { setFrontendSignedOut } from "@/lib/auth-state";

export function ProfilePage() {
  useTitle("Profile");
  const me = useMe();
  const nav = useNavigate();
  const { theme, setTheme } = useTheme();
  const user = me.data?.user || me.data?.principal;

  if (me.isLoading) {
    return (
      <div style={{ maxWidth: "672px", margin: "0 auto", paddingLeft: "24px", paddingRight: "24px", paddingTop: "40px", paddingBottom: "40px" }}>
        <Skeleton style={{ marginBottom: "24px", height: "32px", width: "160px" }} />
        <Skeleton style={{ height: "112px", width: "100%" }} />
      </div>
    );
  }
  if (me.error || !user) {
    return <Center>Could not load your profile.</Center>;
  }
  return (
    <div style={{ maxWidth: "672px", margin: "0 auto", paddingLeft: "24px", paddingRight: "24px", paddingTop: "40px", paddingBottom: "40px" }}>
      <div style={{ marginBottom: "24px" }}>
        <h1 style={{ fontSize: "24px", fontWeight: 600, letterSpacing: "-0.02em" }}>Profile</h1>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
        <Card>
          <CardContent style={{ display: "flex", alignItems: "center", gap: "16px", paddingTop: "20px" }}>
            <Avatar size={56} src={user.picture_url} name={user.name || user.email} />
            <div style={{ minWidth: 0 }}>
              <div style={{ fontWeight: 500 }}>{user.name || "Signed-in user"}</div>
              <div style={{ fontSize: "14px", color: "#71717a" }}>
                {user.email || user.id}
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle style={{ fontSize: "14px" }}>Appearance</CardTitle>
            <CardDescription>Choose how aidocs looks to you.</CardDescription>
          </CardHeader>
          <CardContent>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "8px" }}>
              {(
                [
                  ["light", "Light", Sun],
                  ["dark", "Dark", Moon],
                  ["system", "System", Monitor],
                ] as const
              ).map(([v, label, Icon]) => (
                <button
                  key={v}
                  onClick={() => setTheme(v)}
                  style={{
                    display: "flex",
                    flexDirection: "column",
                    alignItems: "center",
                    gap: "8px",
                    borderRadius: "6px",
                    border: `1px solid ${theme === v ? "#2028ff" : "#e4e4e7"}`,
                    padding: "16px",
                    fontSize: "12px",
                    transition: "background-color 0.15s",
                    background: theme === v ? "#eef0ff" : "transparent",
                    cursor: "pointer",
                  }}
                >
                  <Icon style={{ height: "16px", width: "16px" }} /> {label}
                </button>
              ))}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent style={{ paddingTop: "20px" }}>
            <Button
              variant="outline"
              onClick={() => {
                setFrontendSignedOut(true);
                nav("/");
              }}
            >
              <LogOut style={{ height: "16px", width: "16px" }} /> Sign out
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
