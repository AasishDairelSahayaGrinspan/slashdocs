// Drag-and-drop id convention shared between the documents and folder pages:
// draggable ids are "<DRAG_TYPE>:<resource id>" and droppable ids are
// "drop:<folder id>".
export const DRAG_TYPE = {
  document: "doc",
  folder: "folder",
} as const;

export type ActiveDrag = {
  type: string;
  id: string;
  title: string;
};
