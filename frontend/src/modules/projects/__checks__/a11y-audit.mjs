// Automated DOM accessibility audit for the product UI. It is plain JavaScript that runs INSIDE the page: the headless
// walk injects it with `(${auditPage.toString()})(options)` through the DevTools protocol and reads the returned issues.
// It is a rule-of-thumb audit (names, labels, headings, landmarks, dialogs, keyboard reachability, live regions, contrast),
// not a screen-reader test and not a full WCAG conformance check.

/**
 * @param {{ landmarks?: string[], liveRegion?: boolean, contrast?: boolean }} options
 * @returns {{ rule: string, detail: string }[]}
 */
export function auditPage(options) {
  const opts = options || {};
  const issues = [];
  const add = (rule, detail) => issues.push({ rule, detail: String(detail).slice(0, 160) });
  const describe = (el) => `${el.tagName.toLowerCase()}${el.id ? "#" + el.id : ""}${el.className && typeof el.className === "string" ? "." + el.className.trim().split(/\s+/)[0] : ""} "${(el.textContent || "").trim().slice(0, 30)}"`;
  const visible = (el) => {
    for (let n = el; n && n.nodeType === 1; n = n.parentElement) {
      const cs = getComputedStyle(n);
      if (cs.display === "none" || cs.visibility === "hidden") return false;
      if (n.tagName === "DIALOG" && !n.open) return false;
    }
    return true;
  };
  const textOfIds = (ids) =>
    ids
      .split(/\s+/)
      .map((id) => document.getElementById(id))
      .filter(Boolean)
      .map((n) => n.textContent.trim())
      .join(" ")
      .trim();

  // 1. accessible names and label association
  const nameOf = (el) => {
    const labelledby = el.getAttribute("aria-labelledby");
    if (labelledby && textOfIds(labelledby)) return textOfIds(labelledby);
    const label = (el.getAttribute("aria-label") || "").trim();
    if (label) return label;
    if (el.labels && el.labels.length) {
      const t = [...el.labels].map((l) => l.textContent.trim()).join(" ").trim();
      if (t) return t;
    }
    if (el.tagName === "INPUT" && ["submit", "button", "reset"].includes(el.type)) return (el.value || "").trim();
    if (el.tagName === "IMG") return (el.getAttribute("alt") || "").trim();
    const text = (el.textContent || "").trim();
    if (text) return text;
    return (el.getAttribute("title") || "").trim();
  };
  const interactive = [...document.querySelectorAll("button, a[href], input, select, textarea, summary, [role=button], [role=tab], [role=link]")].filter((el) => !(el.tagName === "INPUT" && el.type === "hidden") && visible(el));
  for (const el of interactive) {
    if (!nameOf(el)) add("accessible-name", `no accessible name: ${describe(el)}`);
    if (["INPUT", "SELECT", "TEXTAREA"].includes(el.tagName) && !el.getAttribute("aria-label") && !el.getAttribute("aria-labelledby") && !(el.labels && el.labels.length)) {
      add("label-association", `form control without a label: ${describe(el)}`);
    }
  }

  // 2. images
  for (const img of document.querySelectorAll("img")) if (visible(img) && !img.hasAttribute("alt")) add("image-alt", `img without alt: ${img.src.slice(0, 60)}`);

  // 3. headings: one h1, no skipped levels
  const headings = [...document.querySelectorAll("h1, h2, h3, h4, h5, h6")].filter(visible);
  const h1s = headings.filter((h) => h.tagName === "H1");
  if (h1s.length !== 1) add("heading-h1", `${h1s.length} visible h1 elements (expected 1)`);
  let prev = 0;
  for (const h of headings) {
    const level = Number(h.tagName[1]);
    if (prev && level > prev + 1) add("heading-order", `h${prev} followed by h${level}: ${describe(h)}`);
    if (!h.textContent.trim()) add("heading-empty", `empty heading ${h.tagName}`);
    prev = level;
  }

  // 4. landmarks
  const required = opts.landmarks || ["banner", "navigation", "main"];
  const has = (role) => {
    const tag = { banner: "header", navigation: "nav", main: "main" }[role];
    return !!document.querySelector(`${tag}, [role=${role}]`);
  };
  for (const role of required) if (!has(role)) add("landmark", `missing ${role} landmark`);
  if (document.querySelectorAll("main, [role=main]").length > 1) add("landmark", "more than one main landmark");

  // 5. dialogs
  for (const d of document.querySelectorAll("dialog")) {
    if (d.getAttribute("role") !== "dialog") add("dialog-role", `dialog without role=dialog: ${describe(d)}`);
    if (d.getAttribute("aria-modal") !== "true") add("dialog-modal", `dialog without aria-modal=true: ${describe(d)}`);
    if (!nameOf(d) || !(d.getAttribute("aria-label") || d.getAttribute("aria-labelledby"))) add("dialog-name", `dialog without an accessible name: ${describe(d)}`);
  }

  // 6. keyboard reachability
  for (const el of document.querySelectorAll("button, a[href], input, select, textarea, [role=button], [role=tab], [role=link]")) {
    if (!visible(el)) continue;
    const ti = el.getAttribute("tabindex");
    if (ti !== null && Number(ti) < 0 && !el.disabled) add("keyboard", `actionable element removed from the tab order: ${describe(el)}`);
    if (el.matches("[role=button], [role=link]") && !el.matches("button, a[href]") && ti === null) add("keyboard", `custom control is not focusable: ${describe(el)}`);
  }
  for (const el of document.querySelectorAll("[onclick]")) if (!el.matches("button, a, input, select, summary, [role=button]")) add("keyboard", `click handler on a non-interactive element: ${describe(el)}`);

  // 7. live region for async messages
  if (opts.liveRegion !== false && !document.querySelector("[aria-live], [role=status], [role=alert]")) add("live-region", "no aria-live / status / alert region for async messages");

  // 8. duplicate ids and dangling references
  const seen = new Set();
  for (const el of document.querySelectorAll("[id]")) {
    if (seen.has(el.id)) add("duplicate-id", `duplicate id ${el.id}`);
    seen.add(el.id);
  }
  for (const el of document.querySelectorAll("[aria-labelledby]")) for (const id of el.getAttribute("aria-labelledby").split(/\s+/)) if (id && !document.getElementById(id)) add("aria-ref", `aria-labelledby points to a missing id ${id}`);

  // 9. contrast of text against its effective background (WCAG 2.x ratio, 4.5:1 normal text, 3:1 large text)
  if (opts.contrast !== false) {
    const parse = (c) => {
      const m = /rgba?\(([^)]+)\)/.exec(c);
      if (!m) return null;
      const p = m[1].split(/[,\s/]+/).filter(Boolean).map(Number);
      return { r: p[0], g: p[1], b: p[2], a: p.length > 3 ? p[3] : 1 };
    };
    const over = (top, bottom) => ({ r: top.r * top.a + bottom.r * (1 - top.a), g: top.g * top.a + bottom.g * (1 - top.a), b: top.b * top.a + bottom.b * (1 - top.a), a: 1 });
    const background = (el) => {
      let color = { r: 255, g: 255, b: 255, a: 0 };
      const layers = [];
      for (let n = el; n && n.nodeType === 1; n = n.parentElement) {
        const cs = getComputedStyle(n);
        if (cs.backgroundImage !== "none" && !cs.backgroundImage.startsWith("linear-gradient(90deg")) return null;
        const bg = parse(cs.backgroundColor);
        if (bg && bg.a > 0) layers.push(bg);
        if (bg && bg.a === 1) break;
      }
      let result = { r: getComputedStyle(document.body).colorScheme.includes("dark") ? 15 : 255, g: getComputedStyle(document.body).colorScheme.includes("dark") ? 20 : 255, b: getComputedStyle(document.body).colorScheme.includes("dark") ? 32 : 255, a: 1 };
      for (const l of layers.reverse()) result = over(l, result);
      color = result;
      return color;
    };
    const lum = (c) => {
      const f = (v) => {
        const s = v / 255;
        return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
      };
      return 0.2126 * f(c.r) + 0.7152 * f(c.g) + 0.0722 * f(c.b);
    };
    const checked = new Set();
    const samples = document.querySelectorAll(".pm-root p, .pm-root span, .pm-root a, .pm-root button, .pm-root label, .pm-root h1, .pm-root h2, .pm-root h3, .pm-root li, .pm-root td, .pm-root th, .pm-root legend, .pm-root summary, .pm-root strong");
    for (const el of samples) {
      if (!visible(el) || el.closest(".pm-sr-only")) continue;
      const own = [...el.childNodes].some((n) => n.nodeType === 3 && n.textContent.trim());
      if (!own) continue;
      const cs = getComputedStyle(el);
      const fg = parse(cs.color);
      const bg = background(el);
      if (!fg || !bg) continue;
      const solid = over(fg, bg);
      const l1 = lum(solid);
      const l2 = lum(bg);
      const ratio = (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05);
      const size = parseFloat(cs.fontSize);
      const large = size >= 24 || (size >= 18.66 && Number(cs.fontWeight) >= 700);
      const need = large ? 3 : 4.5;
      const key = `${cs.color}|${bg.r},${bg.g},${bg.b}|${cs.fontSize}|${(el.className || "").toString().split(/\s+/)[0]}`;
      if (ratio < need && !checked.has(key) && !el.disabled && !(el.closest("button") && el.closest("button").disabled)) {
        checked.add(key);
        add("contrast", `${ratio.toFixed(2)}:1 (< ${need}) ${describe(el)} color ${cs.color} on rgb(${Math.round(bg.r)},${Math.round(bg.g)},${Math.round(bg.b)})`);
      }
    }
  }
  return issues;
}
