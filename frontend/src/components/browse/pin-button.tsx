import { Pin, PinOff } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useTogglePin, type ResourceType } from "./use-resource-actions";

export function PinButton({
  resourceType,
  resourceId,
  pinned,
  className,
}: {
  resourceType: ResourceType;
  resourceId: string;
  pinned?: boolean;
  className: string;
}) {
  const { isPinned, togglePin } = useTogglePin(resourceType, resourceId, pinned);
  return (
    <Button
      variant="ghost"
      size="icon"
      className={className}
      aria-label={isPinned ? "Unpin" : "Pin"}
      onClick={(e) => {
        e.preventDefault();
        e.stopPropagation();
        togglePin.mutate();
      }}
    >
      {isPinned ? <PinOff className="h-3.5 w-3.5" /> : <Pin className="h-3.5 w-3.5" />}
    </Button>
  );
}
