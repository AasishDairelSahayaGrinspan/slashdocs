import * as React from "react";

export const Input = React.forwardRef<
  HTMLInputElement,
  React.InputHTMLAttributes<HTMLInputElement>
>(({ ...props }, ref) => (
  <input
    ref={ref}
    style={{
      height: "36px",
      width: "100%",
      borderRadius: "10px",
      border: "1px solid #e4e4e7",
      background: "#fff",
      padding: "0 12px",
      fontSize: "14px",
      color: "#18181b",
      outline: "none",
    }}
    onFocus={(e) => { e.target.style.boxShadow = "0 0 0 2px #2028ff"; }}
    onBlur={(e) => { e.target.style.boxShadow = "none"; }}
    {...props}
  />
));
Input.displayName = "Input";

export const Textarea = React.forwardRef<
  HTMLTextAreaElement,
  React.TextareaHTMLAttributes<HTMLTextAreaElement>
>(({ ...props }, ref) => (
  <textarea
    ref={ref}
    style={{
      minHeight: "60px",
      width: "100%",
      borderRadius: "10px",
      border: "1px solid #e4e4e7",
      background: "#fff",
      padding: "8px 12px",
      fontSize: "14px",
      color: "#18181b",
      outline: "none",
      resize: "vertical",
    }}
    onFocus={(e) => { e.target.style.boxShadow = "0 0 0 2px #2028ff"; }}
    onBlur={(e) => { e.target.style.boxShadow = "none"; }}
    {...props}
  />
));
Textarea.displayName = "Textarea";

export const Label = React.forwardRef<
  HTMLLabelElement,
  React.LabelHTMLAttributes<HTMLLabelElement>
>(({ ...props }, ref) => (
  <label
    ref={ref}
    style={{ fontSize: "12px", fontWeight: 500, color: "#71717a" }}
    {...props}
  />
));
Label.displayName = "Label";
