import { useState } from "react";
import { Cloud } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { errorMessage } from "@/lib/errors";
import {
  googleDriveDocumentLocation,
  openGoogleDrivePicker,
} from "@/lib/google-drive-picker";

export function GoogleDrivePickerButton({
  connectionId,
  disabled = false,
  onPick,
}: {
  connectionId: string;
  disabled?: boolean;
  onPick: (location: string, name?: string) => void;
}) {
  const [opening, setOpening] = useState(false);
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      disabled={disabled || !connectionId || opening}
      onClick={async () => {
        setOpening(true);
        try {
          const token = await api.getGooglePickerToken(connectionId);
          if (!token.access_token || !token.api_key || !token.app_id) {
            throw new Error("Google Drive Picker is not configured for this workspace");
          }
          const document = await openGoogleDrivePicker({
            accessToken: token.access_token,
            apiKey: token.api_key,
            appId: token.app_id,
          });
          if (!document) return;
          const location = googleDriveDocumentLocation(document);
          if (!location) throw new Error("The selected Drive file has no usable link");
          onPick(location, document.name);
        } catch (error) {
          toast.error(errorMessage(error, "Could not open Google Drive"));
        } finally {
          setOpening(false);
        }
      }}
    >
      <Cloud className="h-3.5 w-3.5" /> {opening ? "Opening…" : "Choose from Drive"}
    </Button>
  );
}
