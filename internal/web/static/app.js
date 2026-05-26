(() => {
  const seenCommandToastKeys = new Set();

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

  function toastStack() {
    return document.getElementById("toast-stack");
  }

  function showToast({ status, title, message, meta }) {
    const stack = toastStack();
    if (!stack) {
      return;
    }

    const toast = document.createElement("article");
    toast.className = `toast ${status || ""}`.trim();
    toast.innerHTML = `
      <div class="toast-head">
        <span class="toast-title"></span>
        <span class="toast-status"></span>
      </div>
      <div class="toast-message"></div>
      <div class="toast-meta"></div>
    `;

    const titleEl = toast.querySelector(".toast-title");
    const statusEl = toast.querySelector(".toast-status");
    const messageEl = toast.querySelector(".toast-message");
    const metaEl = toast.querySelector(".toast-meta");
    if (titleEl) {
      titleEl.textContent = title || "Command";
    }
    if (statusEl) {
      statusEl.textContent = status || "update";
    }
    if (messageEl) {
      messageEl.textContent = message || "";
    }
    if (metaEl) {
      metaEl.textContent = meta || "";
    }

    stack.appendChild(toast);

    const maxToasts = 5;
    while (stack.children.length > maxToasts) {
      stack.removeChild(stack.firstElementChild);
    }

    window.setTimeout(() => {
      toast.classList.add("is-leaving");
      window.setTimeout(() => {
        if (toast.parentElement) {
          toast.parentElement.removeChild(toast);
        }
      }, 220);
    }, 4200);

    toast.addEventListener("click", () => {
      toast.classList.add("is-leaving");
      window.setTimeout(() => {
        if (toast.parentElement) {
          toast.parentElement.removeChild(toast);
        }
      }, 220);
    });

    toast.addEventListener("animationend", (event) => {
      if (event.animationName === "toast-exit" && toast.parentElement) {
        toast.parentElement.removeChild(toast);
      }
    });
  }

  function emitCommandToasts(root = document) {
    const markers = root.querySelectorAll("[data-command-toast='true']");
    markers.forEach((marker) => {
      const key = marker.getAttribute("data-toast-key") || "";
      if (key && seenCommandToastKeys.has(key)) {
        return;
      }
      if (key) {
        seenCommandToastKeys.add(key);
      }

      showToast({
        status: marker.getAttribute("data-toast-status") || "update",
        title: marker.getAttribute("data-toast-title") || "Command",
        message: marker.getAttribute("data-toast-message") || "",
        meta: marker.getAttribute("data-toast-meta") || "",
      });
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
    requestAnimationFrame(() => emitCommandToasts(event.target || document));
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
      emitCommandToasts();
    });
  } else {
    scrollAutoContainersToBottom();
    triggerAutoDownloads();
    emitCommandToasts();
  }
})();
