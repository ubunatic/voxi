// Set dynamic copyright year
const yearEl = document.getElementById("year");
if (yearEl) {
  yearEl.textContent = new Date().getFullYear();
}

// Active link highlighting on scroll
const navLinks = document.querySelectorAll(".nav-links a[href^='#']");
const sections = [...navLinks]
  .map((a) => {
    const selector = a.getAttribute("href");
    return selector && selector.length > 1 ? document.querySelector(selector) : null;
  })
  .filter(Boolean);

if (sections.length && "IntersectionObserver" in window) {
  const observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        const id = `#${entry.target.id}`;
        for (const a of navLinks) {
          a.classList.toggle("active", a.getAttribute("href") === id);
        }
      }
    },
    { rootMargin: "-25% 0px -65% 0px" }
  );
  for (const s of sections) observer.observe(s);
}

// Interactive Copy Buttons
function setupCopyButtons() {
  // Hero terminal copy button
  document.querySelectorAll(".copy-term-btn").forEach((btn) => {
    btn.addEventListener("click", () => {
      const textToCopy = btn.getAttribute("data-copy") || "";
      if (!textToCopy) return;
      navigator.clipboard.writeText(textToCopy).then(() => {
        const orig = btn.textContent;
        btn.textContent = "copied!";
        setTimeout(() => {
          btn.textContent = orig;
        }, 1800);
      });
    });
  });

  // Code block copy buttons
  document.querySelectorAll(".code-block").forEach((block) => {
    const btn = block.querySelector(".copy-code-btn");
    const pre = block.querySelector("pre");
    if (btn && pre) {
      btn.addEventListener("click", () => {
        // Strip out terminal prompts ($) and comments if desired, or copy raw executable lines
        const lines = pre.innerText
          .split("\n")
          .map((l) => l.replace(/^\$\s+/, "").trim())
          .filter((l) => l && !l.startsWith("#"))
          .join("\n");

        navigator.clipboard.writeText(lines || pre.innerText).then(() => {
          const orig = btn.textContent;
          btn.textContent = "copied!";
          setTimeout(() => {
            btn.textContent = orig;
          }, 1800);
        });
      });
    }
  });
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", setupCopyButtons);
} else {
  setupCopyButtons();
}
