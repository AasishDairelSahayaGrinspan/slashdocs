import { Link, useNavigate } from "react-router-dom";
import { Check, LogOut, Monitor, Moon, Rocket, Sun, Terminal, User as UserIcon } from "lucide-react";
import { GetStartedPill } from "./get-started-pill";
import { useOnboarding } from "@/lib/onboarding";
import {
  Dropdown,
  DropdownContent,
  DropdownItem,
  DropdownLabel,
  DropdownSeparator,
  DropdownTrigger,
} from "@/components/ui/dropdown";
import { Avatar } from "@/components/ui/misc";
import { setFrontendSignedOut } from "@/lib/auth-state";
import { useTheme } from "@/lib/theme";
import type { Principal } from "@/api";

export function TopBar({ user }: { user: Principal }) {
  const nav = useNavigate();
  const { theme, setTheme } = useTheme();
  const { reset } = useOnboarding();
  return (
    <header
      style={{
        position: "sticky",
        top: 0,
        zIndex: 30,
        display: "flex",
        height: "56px",
        alignItems: "center",
        justifyContent: "space-between",
        borderBottom: "1px solid #e4e4e7",
        background: "rgba(255,255,255,0.8)",
        backdropFilter: "blur(8px)",
        padding: "0 20px",
      }}
    >
      <Link to="/app/documents" style={{ display: "flex", alignItems: "center", gap: "8px", fontWeight: 600 }}>
        <img src="/favicon.svg" alt="" style={{ height: "40px", width: "40px" }} />
        aidocs
      </Link>
      <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
        <GetStartedPill />
        <Dropdown>
          <DropdownTrigger asChild>
            <button
              style={{ borderRadius: "9999px", border: "none", background: "transparent", cursor: "pointer" }}
              aria-label="Account menu"
            >
              <Avatar src={user.picture_url} name={user.name || user.email || user.id} />
            </button>
          </DropdownTrigger>
          <DropdownContent>
            <div style={{ padding: "8px" }}>
              <div style={{ fontSize: "14px", fontWeight: 500 }}>
                {user.name || "Signed in"}
              </div>
              <div style={{ fontSize: "12px", color: "#71717a" }}>
                {user.email || user.id}
              </div>
            </div>
            <DropdownSeparator />
            <DropdownItem
              onClick={() => {
                reset();
                nav("/app/start");
              }}
            >
              <Rocket width={16} height={16} /> Setup guide
            </DropdownItem>
            <DropdownItem onClick={() => nav("/app/settings/profile")}>
              <UserIcon width={16} height={16} /> Profile
            </DropdownItem>
            <DropdownItem onClick={() => nav("/app/developers")}>
              <Terminal width={16} height={16} /> Developers
            </DropdownItem>
            <DropdownSeparator />
            <DropdownLabel>Theme</DropdownLabel>
            {(
              [
                { value: "light", label: "Light", icon: Sun },
                { value: "dark", label: "Dark", icon: Moon },
                { value: "system", label: "System", icon: Monitor },
              ] as const
            ).map(({ value, label, icon: Icon }) => (
              <DropdownItem
                key={value}
                onClick={(e) => {
                  e.preventDefault();
                  setTheme(value);
                }}
              >
                <Icon width={16} height={16} /> {label}
                {theme === value && <Check style={{ marginLeft: "auto", width: 16, height: 16 }} />}
              </DropdownItem>
            ))}
            <DropdownSeparator />
            <DropdownItem
              onClick={() => {
                setFrontendSignedOut(true);
                nav("/");
              }}
            >
              <LogOut width={16} height={16} /> Sign out
            </DropdownItem>
          </DropdownContent>
        </Dropdown>
      </div>
    </header>
  );
}
