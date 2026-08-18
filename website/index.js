document.getElementById("year").textContent = new Date().getFullYear();

const links = document.querySelectorAll(".nav-links a[href^='#']");
const sections = [...links]
  .map((a) => document.querySelector(a.getAttribute("href")))
  .filter(Boolean);

if (sections.length) {
  const observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        const id = `#${entry.target.id}`;
        for (const a of links) a.classList.toggle("active", a.getAttribute("href") === id);
      }
    },
    { rootMargin: "-40% 0px -55% 0px" }
  );
  for (const s of sections) observer.observe(s);
}
