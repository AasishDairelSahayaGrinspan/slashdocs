import { motion, AnimatePresence } from "framer-motion";
import { FileText } from "lucide-react";

export function MockDocsTable({
  hasRow,
  className,
}: {
  hasRow: boolean;
  className?: string;
}) {
  return (
    <div
      style={{
        overflow: "hidden",
        borderRadius: "12px",
        border: "1px solid #e4e4e7",
        background: "#fff",
        boxShadow: "0 8px 24px rgba(0,0,0,0.12)",
      }}
    >
      <div style={{ borderBottom: "1px solid #e4e4e7", padding: "10px 16px", fontSize: "14px", fontWeight: 600 }}>
        Documents
      </div>
      <div
        style={{
          display: "grid",
          gridTemplateColumns: "1fr 80px 90px",
          gap: "8px",
          borderBottom: "1px solid #e4e4e7",
          background: "rgba(244,244,245,0.5)",
          padding: "8px 16px",
          fontSize: "10px",
          fontWeight: 500,
          textTransform: "uppercase",
          letterSpacing: "0.05em",
          color: "#71717a",
        }}
      >
        <div>Title</div>
        <div>Updated</div>
        <div>Version</div>
      </div>
      <div style={{ minHeight: "140px" }}>
        <AnimatePresence>
          {hasRow && (
            <motion.div
              initial={{ opacity: 0, y: -16, backgroundColor: "rgba(32,40,255,0.12)" }}
              animate={{ opacity: 1, y: 0, backgroundColor: "rgba(32,40,255,0)" }}
              transition={{ duration: 0.5 }}
              style={{
                display: "grid",
                gridTemplateColumns: "1fr 80px 90px",
                alignItems: "center",
                gap: "8px",
                padding: "12px 16px",
                fontSize: "12.5px",
              }}
            >
              <div style={{ display: "flex", alignItems: "center", gap: "8px", overflow: "hidden", textOverflow: "ellipsis" }}>
                <FileText width={14} height={14} style={{ color: "#71717a" }} />
                <span style={{ fontWeight: 500 }}>Q3 incident review</span>
              </div>
              <div style={{ color: "#71717a" }}>just now</div>
              <div>
                <span style={{ borderRadius: "9999px", background: "rgba(238,240,255,1)", padding: "2px 8px", fontSize: "10px", fontWeight: 500, color: "#2028ff" }}>
                  v1
                </span>
              </div>
            </motion.div>
          )}
        </AnimatePresence>
        {!hasRow && (
          <div style={{ display: "flex", height: "140px", alignItems: "center", justifyContent: "center", fontSize: "12px", color: "#71717a" }}>
            No documents yet
          </div>
        )}
      </div>
    </div>
  );
}
