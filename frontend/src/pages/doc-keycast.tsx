import { AnimatePresence, motion } from "framer-motion";
import { CHORD_KEYS, type ChordKey, type ChordState } from "@/lib/esc-chord";

export function Keycast({
  state,
  available,
  label,
}: {
  state: ChordState;
  available: ReadonlyArray<ChordKey>;
  label?: string;
}) {
  const visible = state.phase !== "idle";
  const resolvedKey = state.phase === "resolved" ? state.key : undefined;
  return (
    <AnimatePresence>
      {visible ? (
        <motion.div
          key="keycast"
          initial={{ opacity: 0, y: 6 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: 6 }}
          transition={{ duration: 0.15, ease: "easeOut" }}
          className="pointer-events-none absolute bottom-6 left-5 z-30 select-none"
          aria-hidden="true"
        >
          <div className="inline-flex items-center gap-2 rounded-xl bg-neutral-700/95 px-4 py-2.5 font-mono text-lg font-medium tracking-wide text-white shadow-lg dark:bg-neutral-800/95">
            <span>ESC</span>
            {resolvedKey ? (
              <>
                <span className="text-white/50">+</span>
                <span>{resolvedKey}</span>
              </>
            ) : null}
          </div>
          <div className="mt-1.5 pl-1 text-xs text-[var(--color-fg-muted)]">
            {resolvedKey
              ? label
              : CHORD_KEYS.filter((item) => available.includes(item.key)).map(
                  (item) => (
                    <span
                      key={item.key}
                      className="mr-3 inline-block whitespace-nowrap"
                    >
                      <span className="font-mono font-medium text-[var(--color-fg)]">
                        {item.key}
                      </span>{" "}
                      {item.hint}
                    </span>
                  ),
                )}
          </div>
        </motion.div>
      ) : null}
    </AnimatePresence>
  );
}
