// Dev-only browser driver for /dev-test: lets an automated browser click "Run scenario" and read the step tables.
window.setVal = (el, v) => {
  const p = el.tagName === "SELECT" ? HTMLSelectElement.prototype : HTMLInputElement.prototype;
  Object.getOwnPropertyDescriptor(p, "value").set.call(el, v);
  el.dispatchEvent(new Event(el.tagName === "SELECT" ? "change" : "input", { bubbles: true }));
};
window.runAll = async (tabLabel, only) => {
  [...document.querySelectorAll(".dt-tabs button")].find((x) => x.textContent.startsWith(tabLabel)).click();
  await new Promise((r) => setTimeout(r, 300));
  const out = [];
  const cards = [...document.querySelectorAll(".dt-card")].filter((c) => c.querySelector(".dt-actions button")?.textContent.match(/Run scenario|Running/) && c.offsetParent !== null);
  for (const c of cards) {
    const title = c.querySelector(".dt-chead b").textContent;
    if (only && !title.toLowerCase().includes(only)) continue;
    const btn = c.querySelector(".dt-actions button");
    if (btn.disabled) { out.push(title + ": SKIPPED (disabled) " + (c.querySelector(".dt-warn")?.textContent ?? "")); continue; }
    btn.click();
    await new Promise((r) => setTimeout(r, 500));
    for (let i = 0; i < 240 && c.querySelector(".dt-actions button").textContent === "Running…"; i++) await new Promise((r) => setTimeout(r, 500));
    const rows = [...c.querySelectorAll("tbody > tr.dt-click, tbody > tr:has(.dt-badge)")].map((tr) => [...tr.children].map((td) => td.innerText.replace(/\n.*/s, "")).slice(1).join(" | "));
    out.push("## " + title + " => " + c.querySelector(".dt-chead .dt-badge").textContent + "\n" + rows.join("\n"));
  }
  return out.join("\n\n");
};
window.failsOnly = (s) => s.split("\n").filter((l) => l.startsWith("##") || l.includes("| FAIL |")).join("\n");
