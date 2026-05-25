(() => {
  function triggerAutoDownloads(root = document) {
    const links = root.querySelectorAll('a[data-auto-download="true"]');
    links.forEach((link) => {
      if (link.dataset.downloadTriggered === "true") {
        return;
      }
      link.dataset.downloadTriggered = "true";
      link.click();
    });
  }

  function scrollAutoContainersToBottom(root = document) {
    const containers = root.querySelectorAll('[data-autoscroll="bottom"]');
    containers.forEach((container) => {
      container.scrollTop = container.scrollHeight;
    });
  }

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
    const envButton = event.target.closest(".env-button[data-env-button='true']");
    if (envButton) {
      const selectedEnvironment = envButton.getAttribute("data-environment");
      document.querySelectorAll(".env-button[data-env-button='true']").forEach((button) => {
        button.classList.remove("active");
      });
      envButton.classList.add("active");

      const logsTabButton = document.getElementById("logs-tab-button");
      if (logsTabButton && selectedEnvironment) {
        const basePath = logsTabButton.getAttribute("hx-get").split("?")[0];
        logsTabButton.setAttribute("hx-get", `${basePath}?environment=${encodeURIComponent(selectedEnvironment)}`);
      }

      const commitsTabButton = document.getElementById("commits-tab-button");
      if (commitsTabButton && selectedEnvironment) {
        const basePath = commitsTabButton.getAttribute("hx-get").split("?")[0];
        commitsTabButton.setAttribute("hx-get", `${basePath}?environment=${encodeURIComponent(selectedEnvironment)}`);
      }

      setActiveTab("logs");
      return;
    }

    const button = event.target.closest("[data-tab-link]");
    if (!button) {
      return;
    }
    setActiveTab(button.getAttribute("data-tab-link"));
  });

  document.body.addEventListener("htmx:afterSwap", (event) => {
    requestAnimationFrame(() => triggerAutoDownloads(event.target || document));
    requestAnimationFrame(() => scrollAutoContainersToBottom(event.target || document));
    if (event.target && event.target.id === "tab-panel") {
      const active = document.querySelector("[data-tab-link].active");
      if (!active) {
        if (document.querySelector("[data-tab-link='commits']")) {
          setActiveTab("commits");
        } else {
          setActiveTab("events");
        }
      }
    }
  });

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => {
      scrollAutoContainersToBottom();
      triggerAutoDownloads();
    });
  } else {
    scrollAutoContainersToBottom();
    triggerAutoDownloads();
  }
})();
