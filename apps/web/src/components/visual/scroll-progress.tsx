"use client";

import { useEffect, useState } from "react";

/**
 * ScrollProgress —— 视口顶部细进度条，跟随当前滚动百分比。
 *
 * 不是「现在加载到几 %」，而是「当前页面已经向下滚动多少」—— 让长表/日志页
 * 第一眼就知道「这页还有多少」。粒度细（实际 px 转 transform），不会卡。
 *
 * 自动只在内容比视口高出 ≥ 200px 时显示，避免短页底部多一道线。
 */
export function ScrollProgress() {
  const [pct, setPct] = useState(0);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    function compute() {
      const docH = document.documentElement.scrollHeight;
      const viewH = window.innerHeight;
      const overflow = docH - viewH;
      if (overflow < 200) {
        setVisible(false);
        return;
      }
      setVisible(true);
      const p = Math.min(100, Math.max(0, (window.scrollY / overflow) * 100));
      setPct(p);
    }
    compute();
    window.addEventListener("scroll", compute, { passive: true });
    window.addEventListener("resize", compute);
    // re-check on DOM changes (route swap, content load)
    const obs = new MutationObserver(compute);
    obs.observe(document.body, { childList: true, subtree: true });
    return () => {
      window.removeEventListener("scroll", compute);
      window.removeEventListener("resize", compute);
      obs.disconnect();
    };
  }, []);

  if (!visible) return null;
  return (
    <div
      aria-hidden
      className="pointer-events-none fixed inset-x-0 top-0 z-30 h-0.5 bg-transparent"
    >
      <div
        className="h-full origin-left bg-gradient-to-r from-srapi-primary via-srapi-primary to-srapi-warning shadow-[0_0_6px_color-mix(in_oklab,var(--color-srapi-primary)_60%,transparent)] transition-[transform] duration-150 ease-out"
        style={{ transform: `scaleX(${pct / 100})` }}
      />
    </div>
  );
}
