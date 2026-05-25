(() => {
  function setActiveTab(linkValue) {
    const links = document.querySelectorAll("[data-tab-link]");
    links.forEach((el) => {
      if (el.getAttribute("data-tab-link") === linkValue) {
        el.classList.add("active");
      } else if (el.classList.contains("tab") || el.classList.contains("env-button")) {
        el.classList.remove("active");
      }
    });
  }

  document.body.addEventListener("click", (event) => {
    const button = event.target.closest("[data-tab-link]");
    if (!button) {
      return;
    }
    setActiveTab(button.getAttribute("data-tab-link"));
  });

  document.body.addEventListener("htmx:afterSwap", (event) => {
    if (event.target && event.target.id === "tab-panel") {
      const active = document.querySelector("[data-tab-link].active");
      if (!active) {
        setActiveTab("events");
      }
    }
  });
})();
