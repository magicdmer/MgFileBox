(function () {
  const $ = (selector, scope = document) => scope.querySelector(selector);
  const $$ = (selector, scope = document) => Array.from(scope.querySelectorAll(selector));

  function setMessage(element, text, kind) {
    if (!element) return;
    element.hidden = !text;
    element.textContent = text || "";
    element.classList.remove("is-error", "is-success");
    if (kind === "error") element.classList.add("is-error");
    if (kind === "success") element.classList.add("is-success");
  }

  function bindLoginForm() {
    const form = $("#login-form");
    if (!form) return;

    const message = $("#login-message");
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      setMessage(message, "", "");

      const formData = new FormData(form);
      const response = await fetch(form.action, {
        method: "POST",
        body: formData,
      });

      const payload = await response.json();
      if (!response.ok || !payload.success) {
        setMessage(message, payload.message || "登录失败", "error");
        return;
      }

      window.location.href = payload.redirect || "/upload";
    });
  }

  function bindDropzone() {
    const dropzone = $("#dropzone");
    const fileInput = $("#file-input");
    const fileName = $("#file-name");
    const selectedFilesPanel = $("#selected-files");
    const selectedFilesList = $("#selected-files-list");
    const selectedFilesSummary = $("#selected-files-summary");
    const clearSelectedFiles = $("#clear-selected-files");
    if (!dropzone || !fileInput || !fileName || !selectedFilesPanel || !selectedFilesList || !selectedFilesSummary || !clearSelectedFiles) return;

    let selectedFiles = [];

    const updateUploadProgress = (loaded, total) => {
      let remaining = total > 0 ? Math.min(loaded / total, 1) * selectedFiles.reduce((sum, file) => sum + file.size, 0) : 0;

      $$(".selected-file-row", selectedFilesList).forEach((row, index) => {
        const fileSize = selectedFiles[index] ? selectedFiles[index].size : 0;
        const progress = fileSize > 0 ? Math.min(remaining / fileSize, 1) : 0;
        row.style.setProperty("--upload-progress", `${progress * 100}%`);
        remaining = Math.max(remaining - fileSize, 0);
      });
    };

    const formatFileSize = (size) => {
      if (size < 1024) return `${size} B`;
      if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
      if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`;
      return `${(size / 1024 / 1024 / 1024).toFixed(1)} GB`;
    };

    const syncInputFiles = () => {
      const transfer = new DataTransfer();
      selectedFiles.forEach((file) => transfer.items.add(file));
      fileInput.files = transfer.files;
    };

    const updateLabel = () => {
      if (selectedFiles.length === 0) {
        fileName.textContent = "点击选择文件，也可以拖拽到这里";
        return;
      }
      if (selectedFiles.length === 1) {
        fileName.textContent = selectedFiles[0].name;
        return;
      }
      fileName.textContent = `已选择 ${selectedFiles.length} 个文件`;
    };

    const renderSelectedFiles = () => {
      selectedFilesPanel.hidden = selectedFiles.length === 0;
      const totalSize = selectedFiles.reduce((sum, file) => sum + file.size, 0);
      selectedFilesSummary.textContent = `${selectedFiles.length} 个文件 · ${formatFileSize(totalSize)}`;
      selectedFilesList.replaceChildren();

      selectedFiles.forEach((file, index) => {
        const row = document.createElement("div");
        row.className = "selected-file-row";
        row.style.setProperty("--upload-progress", "0%");

        const progress = document.createElement("div");
        progress.className = "selected-file-row__progress";
        progress.setAttribute("aria-hidden", "true");

        const main = document.createElement("div");
        main.className = "selected-file-row__main";
        main.innerHTML = '<svg aria-hidden="true" width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline></svg>';

        const name = document.createElement("span");
        name.className = "selected-file-row__name";
        name.textContent = file.name;
        name.title = file.name;
        main.appendChild(name);

        const size = document.createElement("span");
        size.className = "selected-file-row__size";
        size.textContent = formatFileSize(file.size);

        const remove = document.createElement("button");
        remove.type = "button";
        remove.className = "selected-file-row__remove";
        remove.setAttribute("aria-label", `移除 ${file.name}`);
        remove.title = "移除文件";
        remove.innerHTML = '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6L6 18M6 6l12 12"></path></svg>';
        remove.addEventListener("click", () => {
          selectedFiles.splice(index, 1);
          syncInputFiles();
          renderSelectedFiles();
          updateLabel();
        });

        row.append(progress, main, size, remove);
        selectedFilesList.appendChild(row);
      });
    };

    const addFiles = (files) => {
      const signatures = new Set(selectedFiles.map((file) => `${file.name}:${file.size}:${file.lastModified}`));
      files.forEach((file) => {
        const signature = `${file.name}:${file.size}:${file.lastModified}`;
        if (signatures.has(signature)) return;
        signatures.add(signature);
        selectedFiles.push(file);
      });
      syncInputFiles();
      renderSelectedFiles();
      updateLabel();
    };

    fileInput.addEventListener("change", () => {
      addFiles(Array.from(fileInput.files || []));
    });

    ["dragenter", "dragover"].forEach((eventName) => {
      dropzone.addEventListener(eventName, (event) => {
        event.preventDefault();
        dropzone.classList.add("is-dragover");
      });
    });

    ["dragleave", "drop"].forEach((eventName) => {
      dropzone.addEventListener(eventName, (event) => {
        event.preventDefault();
        dropzone.classList.remove("is-dragover");
      });
    });

    dropzone.addEventListener("drop", (event) => {
      const droppedFiles = event.dataTransfer && event.dataTransfer.files;
      if (!droppedFiles || droppedFiles.length === 0) return;

      addFiles(Array.from(droppedFiles));
    });

    clearSelectedFiles.addEventListener("click", () => {
      selectedFiles = [];
      syncInputFiles();
      renderSelectedFiles();
      updateLabel();
    });

    const form = fileInput.closest("form");
    if (form) {
      form.updateUploadProgress = updateUploadProgress;
      form.addEventListener("reset", () => {
        selectedFiles = [];
        syncInputFiles();
        renderSelectedFiles();
        updateLabel();
      });
    }
  }

  function bindCustomSelect() {
    const select = $("#expires-select");
    if (!select) return;

    const trigger = $(".custom-select__trigger", select);
    const valueDisplay = $(".custom-select__value", select);
    const hiddenInput = $('input[name="expires_hours"]', select);
    const options = $$(".custom-select__option", select);

    trigger.addEventListener("click", () => {
      select.classList.toggle("is-open");
    });

    options.forEach(option => {
      option.addEventListener("click", () => {
        // 更新值
        const value = option.dataset.value;
        const text = option.textContent;
        hiddenInput.value = value;
        valueDisplay.textContent = text;

        // 更新选中状态样式
        options.forEach(opt => opt.classList.remove("is-selected"));
        option.classList.add("is-selected");

        // 关闭下拉框
        select.classList.remove("is-open");
      });
    });

    // 点击外部区域关闭下拉框
    document.addEventListener("click", (event) => {
      if (!select.contains(event.target)) {
        select.classList.remove("is-open");
      }
    });
  }

  function bindUploadForm() {
    const form = $("#share-form");
    if (!form) return;

    const resultCard = $("#upload-result");
    const shareUrlInput = $("#share-url");
    const visitLink = $("#share-visit-link");
    const message = $("#upload-message");
    const resetButton = $("#reset-share-form");
    const submitButton = $("button[type=\"submit\"]", form);
    const submitLabel = submitButton.textContent;
    let activeRequest = null;
    let isUploading = false;

    const getUploadErrorMessage = (request, responsePayload) => {
      let redirectedToLogin = false;
      try {
        redirectedToLogin = new URL(request.responseURL, window.location.href).pathname === "/login";
      } catch (error) {
        redirectedToLogin = false;
      }

      if (request.status === 401 || request.status === 403 || redirectedToLogin) {
        return "登录状态已失效，请重新登录";
      }
      if (responsePayload && typeof responsePayload.message === "string" && responsePayload.message.trim()) {
        return responsePayload.message;
      }
      if (request.status === 413) {
        return "文件总大小超过服务器允许的上传上限，请减小文件后重试";
      }
      if (request.status === 408 || request.status === 504) {
        return "上传超时，请检查网络后重试";
      }
      if (request.status === 502 || request.status === 503) {
        return "上传服务暂时不可用，请稍后重试";
      }
      if (request.status >= 500) {
        return "服务器处理上传时发生错误，请稍后重试";
      }
      return "创建分享失败，请稍后重试";
    };

    const setSubmitState = (state) => {
      submitButton.classList.toggle("button-primary", state !== "uploading");
      submitButton.classList.toggle("button-danger-solid", state === "uploading");

      if (state === "uploading") {
        submitButton.disabled = false;
        submitButton.textContent = "取消上传";
        return;
      }
      if (state === "processing") {
        submitButton.disabled = true;
        submitButton.textContent = "正在生成分享链接…";
        return;
      }

      submitButton.disabled = false;
      submitButton.textContent = submitLabel;
    };

    form.addEventListener("submit", async (event) => {
      event.preventDefault();

      if (activeRequest && isUploading) {
        activeRequest.abort();
        return;
      }

      setMessage(message, "", "");
      setSubmitState("uploading");

      try {
        const data = new FormData(form);
        const payload = await new Promise((resolve, reject) => {
          const request = new XMLHttpRequest();
          activeRequest = request;
          isUploading = true;
          request.open("POST", "/api/shares/file");
          request.responseType = "json";
          request.upload.addEventListener("progress", (progressEvent) => {
            if (progressEvent.lengthComputable && form.updateUploadProgress) {
              form.updateUploadProgress(progressEvent.loaded, progressEvent.total);
            }
          });
          request.upload.addEventListener("load", () => {
            isUploading = false;
            setSubmitState("processing");
          });
          request.addEventListener("load", () => {
            const responsePayload = request.response || {};
            if (request.status < 200 || request.status >= 300 || !responsePayload.success) {
              resolve({ success: false, message: getUploadErrorMessage(request, responsePayload) });
              return;
            }
            if (form.updateUploadProgress) form.updateUploadProgress(1, 1);
            resolve(responsePayload);
          });
          request.addEventListener("abort", () => {
            const error = new Error("upload cancelled");
            error.name = "AbortError";
            reject(error);
          });
          request.addEventListener("error", () => {
            const error = new Error("network error");
            error.name = "NetworkError";
            reject(error);
          });
          request.send(data);
        });

        if (!payload.success) {
          if (form.updateUploadProgress) form.updateUploadProgress(0, 1);
          setMessage(message, payload.message || "创建分享失败", "error");
          resultCard.hidden = true;
          return;
        }

        const shareURL = payload.data.shareUrl;
        shareUrlInput.value = shareURL;
        visitLink.href = shareURL;
        form.hidden = true;
        resultCard.hidden = false;
        setMessage(message, "", "");
        resultCard.scrollIntoView({ behavior: "smooth", block: "nearest" });
      } catch (error) {
        if (form.updateUploadProgress) form.updateUploadProgress(0, 1);
        if (error && error.name === "AbortError") {
          setMessage(message, "已取消上传", "");
        } else if (error && error.name === "NetworkError") {
          setMessage(message, "网络连接中断，请检查网络后重试", "error");
        } else {
          setMessage(message, "上传中断，请确认文件未被移动、覆盖保存或同步修改，然后重新选择文件再试", "error");
        }
        resultCard.hidden = true;
      } finally {
        activeRequest = null;
        isUploading = false;
        setSubmitState("idle");
      }
    });

    resetButton.addEventListener("click", () => {
      form.reset();
      $("#file-name").textContent = "点击选择文件，也可以拖拽到这里";
      form.hidden = false;
      resultCard.hidden = true;
      setMessage(message, "", "");
    });
  }

  async function copyText(text, trigger) {
    try {
      await navigator.clipboard.writeText(text);
      if (!trigger) return;
      const oldHTML = trigger.innerHTML;
      trigger.textContent = "已复制";
      setTimeout(() => {
        trigger.innerHTML = oldHTML;
      }, 1200);
    } catch (error) {
      showNoticeDialog("复制失败，请手动复制");
    }
  }

  function showNoticeDialog(text) {
    let modal = $("[data-notice-modal]");
    if (!modal) {
      modal = document.createElement("div");
      modal.className = "modal-backdrop";
      modal.dataset.noticeModal = "";
      modal.hidden = true;

      const dialog = document.createElement("section");
      dialog.className = "confirm-dialog";
      dialog.setAttribute("role", "dialog");
      dialog.setAttribute("aria-modal", "true");
      dialog.setAttribute("aria-labelledby", "notice-dialog-title");
      dialog.setAttribute("aria-describedby", "notice-dialog-description");

      const icon = document.createElement("div");
      icon.className = "confirm-dialog__icon confirm-dialog__icon--neutral";
      icon.setAttribute("aria-hidden", "true");
      icon.innerHTML = `<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><path d="M12 16v-4"></path><path d="M12 8h.01"></path></svg>`;

      const content = document.createElement("div");
      content.className = "confirm-dialog__content";
      const title = document.createElement("h2");
      title.id = "notice-dialog-title";
      title.textContent = "操作未完成";
      const description = document.createElement("p");
      description.id = "notice-dialog-description";
      description.dataset.noticeModalText = "";
      content.append(title, description);

      const actions = document.createElement("div");
      actions.className = "confirm-dialog__actions";
      const okButton = document.createElement("button");
      okButton.type = "button";
      okButton.className = "button button-primary button-compact";
      okButton.dataset.noticeModalClose = "";
      okButton.textContent = "知道了";
      actions.append(okButton);

      dialog.append(icon, content, actions);
      modal.append(dialog);
      document.body.append(modal);

      const close = () => {
        modal.hidden = true;
        if (modal.lastFocused) modal.lastFocused.focus();
      };
      okButton.addEventListener("click", close);
      modal.addEventListener("click", (event) => {
        if (event.target === modal) close();
      });
      document.addEventListener("keydown", (event) => {
        if (event.key === "Escape" && !modal.hidden) close();
      });
    }

    const description = $("[data-notice-modal-text]", modal);
    const closeButton = $("[data-notice-modal-close]", modal);
    if (description) description.textContent = text;
    modal.lastFocused = document.activeElement;
    modal.hidden = false;
    if (closeButton) closeButton.focus();
  }

  function bindCopyButtons() {
    $$("[data-copy-target]").forEach((button) => {
      button.addEventListener("click", () => {
        const target = document.getElementById(button.dataset.copyTarget);
        if (!target) return;
        copyText(target.value, button);
      });
    });

    $$("[data-copy-value]").forEach((button) => {
      button.addEventListener("click", () => {
        copyText(button.dataset.copyValue || "", button);
      });
    });
  }

  function bindDeleteButtons() {
    const modal = $("[data-delete-modal]");
    const title = $("[data-delete-modal-title]");
    const description = $("[data-delete-modal-description]");
    const cancelButton = $("[data-delete-modal-cancel]");
    const confirmButton = $("[data-delete-modal-confirm]");
    const message = $("[data-delete-modal-message]");
    let pendingButton = null;
    let lastFocused = null;

    if (!modal || !cancelButton || !confirmButton) return;

    const closeModal = () => {
      modal.hidden = true;
      pendingButton = null;
      setMessage(message, "", "");
      confirmButton.disabled = false;
      confirmButton.textContent = "删除";
      if (lastFocused) lastFocused.focus();
    };

    const openModal = (button) => {
      pendingButton = button;
      lastFocused = document.activeElement;
      setMessage(message, "", "");
      if (title) title.textContent = "删除这条分享？";
      if (description) description.textContent = "删除后链接会立即失效，本地文件也会被移除。分享记录会保留在已删除列表中。";
      confirmButton.textContent = "删除";
      modal.hidden = false;
      cancelButton.focus();
    };

    const moveDeletedItem = (button) => {
      const item = button.closest("[data-share-id]");
      if (!item) return;
      const shareId = button.dataset.deleteShare;

      const deletedItem = item.cloneNode(true);
      deletedItem.removeAttribute("data-share-id");
      deletedItem.classList.add("share-item--deleted");
      let actions = $(".share-item__actions", deletedItem);
      if (actions) {
        $$("[data-copy-value], [data-delete-share]", actions).forEach((control) => control.remove());
      } else {
        actions = document.createElement("div");
        actions.className = "share-item__actions";
        const main = $(".share-item__main", deletedItem);
        if (main) main.insertAdjacentElement("afterend", actions);
      }
      const purgeButton = createPurgeButton(shareId);
      purgeButton.addEventListener("click", () => purgeDeletedShare(purgeButton));
      actions.append(purgeButton);
      const status = $(".badge-active, .badge-expired", deletedItem);
      if (status) {
        status.className = "badge badge-deleted";
        status.textContent = "已删除";
      }
      const files = $(".share-files", deletedItem);
      const toggleButton = $("[data-toggle-files]", deletedItem);
      if (files && toggleButton) {
        const deletedFilesID = `files-deleted-${shareId}`;
        files.id = deletedFilesID;
        files.hidden = true;
        toggleButton.setAttribute("aria-controls", deletedFilesID);
        toggleButton.setAttribute("aria-expanded", "false");
        const label = $("span", toggleButton);
        if (label) label.textContent = "展开";
        toggleButton.addEventListener("click", () => toggleFileDisclosure(toggleButton));
      }
      const expiryTime = $("[data-share-expiry-time]", deletedItem);
      if (expiryTime) {
        const now = new Date();
        const pad = (value) => String(value).padStart(2, "0");
        expiryTime.removeAttribute("data-share-expiry-time");
        expiryTime.setAttribute("data-share-deleted-time", "");
        expiryTime.textContent = `删除：${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())} ${pad(now.getHours())}:${pad(now.getMinutes())}`;
      }
      const deletedList = $("[data-deleted-share-list]");
      if (deletedList) deletedList.prepend(deletedItem);
      const deletedEmpty = $("[data-deleted-empty]");
      if (deletedEmpty) deletedEmpty.hidden = true;
      const deletedCount = $("[data-tab-count=\"deleted\"]");
      if (deletedCount) deletedCount.textContent = String(Number(deletedCount.textContent || "0") + 1);
      item.remove();

      const remaining = $$("[data-active-share-list] [data-share-id]").length;
      const count = $("[data-tab-count=\"active\"]");
      if (count) count.textContent = String(remaining);
      const emptyState = $("[data-active-empty]");
      if (emptyState) emptyState.hidden = remaining !== 0;
    };

    const removePurgedItem = (button) => {
      const item = button.closest(".share-item");
      if (item) item.remove();

      const remaining = $$("[data-deleted-share-list] .share-item").length;
      const deletedCount = $("[data-tab-count=\"deleted\"]");
      if (deletedCount) deletedCount.textContent = String(remaining);
      const deletedEmpty = $("[data-deleted-empty]");
      if (deletedEmpty) deletedEmpty.hidden = remaining !== 0;
    };

    const deleteShare = async () => {
      if (!pendingButton) return;
      const shareId = pendingButton.dataset.deleteShare;
      if (!shareId) return;

      confirmButton.disabled = true;
      confirmButton.textContent = "删除中…";
      setMessage(message, "", "");

      try {
        const response = await fetch(`/api/admin/shares/${shareId}/delete`, {
          method: "POST",
        });
        const payload = await response.json();
        if (!response.ok || !payload.success) {
          setMessage(message, payload.message || "删除失败", "error");
          confirmButton.disabled = false;
          confirmButton.textContent = "删除";
          return;
        }

        moveDeletedItem(pendingButton);
        closeModal();
      } catch (error) {
        setMessage(message, "网络异常，请稍后重试", "error");
        confirmButton.disabled = false;
        confirmButton.textContent = "删除";
      }
    };

    const createPurgeButton = (shareId) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "button button-danger button-icon";
      button.dataset.purgeShare = shareId;
      button.setAttribute("aria-label", "清除记录");
      button.title = "清除记录";
      button.innerHTML = `<svg aria-hidden="true" width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18"></path><path d="M8 6V4h8v2"></path><path d="M19 6l-1 14H6L5 6"></path><path d="M10 11v5M14 11v5"></path></svg>`;
      return button;
    };

    const purgeDeletedShare = async (button) => {
      const shareId = button.dataset.purgeShare;
      if (!shareId) return;

      button.disabled = true;
      try {
        const response = await fetch(`/api/admin/shares/${shareId}/purge`, {
          method: "POST",
        });
        const payload = await response.json();
        if (!response.ok || !payload.success) {
          showNoticeDialog(payload.message || "清除记录失败");
          button.disabled = false;
          return;
        }
        removePurgedItem(button);
      } catch (error) {
        showNoticeDialog("网络异常，请稍后重试");
        button.disabled = false;
      }
    };

    $$("[data-delete-share]").forEach((button) => {
      button.addEventListener("click", () => openModal(button));
    });
    $$("[data-purge-share]").forEach((button) => {
      button.addEventListener("click", () => purgeDeletedShare(button));
    });
    cancelButton.addEventListener("click", closeModal);
    confirmButton.addEventListener("click", deleteShare);
    modal.addEventListener("click", (event) => {
      if (event.target === modal) closeModal();
    });
    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && !modal.hidden) closeModal();
    });
  }

  function bindManagementTabs() {
    const buttons = $$("[data-management-tab]");
    if (buttons.length === 0) return;

    buttons.forEach((button) => {
      button.addEventListener("click", () => {
        const target = button.dataset.managementTab;
        buttons.forEach((item) => {
          const active = item === button;
          item.classList.toggle("is-active", active);
          item.setAttribute("aria-selected", String(active));
        });
        $$("[data-management-panel]").forEach((panel) => {
          const active = panel.dataset.managementPanel === target;
          panel.classList.toggle("is-active", active);
          panel.hidden = !active;
        });
      });
    });
  }

  function bindFileDisclosures() {
    $$("[data-toggle-files]").forEach((button) => {
      button.addEventListener("click", () => toggleFileDisclosure(button));
    });
  }

  function bindDownloadCounters() {
    $$('[data-download-count]').forEach((link) => {
      link.addEventListener("click", () => {
        const countURL = link.dataset.downloadCount;
        if (!countURL) return;

        if (navigator.sendBeacon) {
          navigator.sendBeacon(countURL, new Blob([], { type: "application/octet-stream" }));
          return;
        }

        fetch(countURL, { method: "POST", keepalive: true }).catch(() => {});
      });
    });
  }

  function toggleFileDisclosure(button) {
    const targetID = button.getAttribute("aria-controls") || `files-${button.dataset.toggleFiles}`;
    const target = document.getElementById(targetID);
    if (!target) return;
    const expanded = button.getAttribute("aria-expanded") === "true";
    button.setAttribute("aria-expanded", String(!expanded));
    target.hidden = expanded;
    const label = $("span", button);
    if (label) label.textContent = expanded ? "展开" : "收起";
  }

  bindLoginForm();
  bindDropzone();
  bindCustomSelect();
  bindUploadForm();
  bindCopyButtons();
  bindDeleteButtons();
  bindDownloadCounters();
  bindFileDisclosures();
  bindManagementTabs();
})();
