"use client";

import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { usePathname } from "next/navigation";

/**
 * PageTransition —— 路由切换时的「卡片化淡入位移」。
 *
 * 替代 AppShell 原本仅用 opacity 的页面进场，加入 4px 位移让节奏更明显，
 * 但克制（不喧宾夺主）。使用 framer-motion 的 AnimatePresence + key=pathname
 * 让 Next App Router 的客户端导航也走过渡。
 *
 * 尊重 prefers-reduced-motion：直接跳到终态。
 */
export function PageTransition({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const reduce = useReducedMotion();
  if (reduce) {
    return <>{children}</>;
  }
  return (
    <AnimatePresence mode="popLayout" initial={false}>
      <motion.div
        key={pathname}
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        transition={{ duration: 0.15, ease: "easeOut" }}
      >
        {children}
      </motion.div>
    </AnimatePresence>
  );
}
