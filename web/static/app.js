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

  function showToast(text, kind = "error") {
    const previous = $("[data-toast]");
    if (previous) previous.remove();
    const toast = document.createElement("div");
    toast.className = `app-toast${kind === "error" ? " is-error" : ""}`;
    toast.dataset.toast = "";
    toast.setAttribute("role", kind === "error" ? "alert" : "status");
    toast.setAttribute("aria-atomic", "true");
    const icon = document.createElement("span");
    icon.className = "app-toast__icon";
    icon.innerHTML = '<svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"></circle><path d="M12 7v6M12 17h.01"></path></svg>';
    const message = document.createElement("span");
    message.className = "app-toast__message";
    message.textContent = text;
    const close = document.createElement("button");
    close.type = "button";
    close.className = "app-toast__close";
    close.setAttribute("aria-label", "关闭提示");
    close.title = "关闭提示";
    close.innerHTML = '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="m6 6 12 12M18 6 6 18"></path></svg>';
    toast.append(icon, message, close);
    let timer;
    const dismiss = () => { clearTimeout(timer); toast.remove(); };
    const scheduleDismiss = () => {
      clearTimeout(timer);
      if (!toast.contains(document.activeElement)) timer = setTimeout(dismiss, 4500);
    };
    close.addEventListener("click", dismiss);
    toast.addEventListener("pointerenter", () => clearTimeout(timer));
    toast.addEventListener("pointerleave", scheduleDismiss);
    toast.addEventListener("focusin", () => clearTimeout(timer));
    toast.addEventListener("focusout", () => setTimeout(scheduleDismiss, 0));
    document.body.append(toast);
    scheduleDismiss();
    return dismiss;
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
    const sizeLimit = $(".upload-size-limit", dropzone);
    const selectedFilesPanel = $("#selected-files");
    const selectedFilesList = $("#selected-files-list");
    const selectedFilesSummary = $("#selected-files-summary");
    const clearSelectedFiles = $("#clear-selected-files");
    if (!dropzone || !fileInput || !fileName || !selectedFilesPanel || !selectedFilesList || !selectedFilesSummary || !clearSelectedFiles) return;
    const form = fileInput.closest("form");
    let dismissSizeToast = null;

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
      if (form && form.validateSelectedFiles) form.validateSelectedFiles();
    };

    const updateLabel = () => {
      if (sizeLimit) sizeLimit.hidden = selectedFiles.length > 0;
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

    if (form) {
      form.updateUploadProgress = updateUploadProgress;
      form.validateSelectedFiles = () => {
        const limit = Number(form.dataset.maxUploadSize);
        const totalSize = Array.from(fileInput.files || []).reduce((sum, file) => sum + file.size, 0);
        const text = limit > 0 && totalSize > limit
          ? `上传的文件超过了 ${formatFileSize(limit).replace(/\.0(?=\s)/, "").replace(/\b(K|M|G)B\b/, "$1")} 上限`
          : "";
        if (text) {
          dismissSizeToast = showToast(text);
        } else if (dismissSizeToast) {
          dismissSizeToast();
          dismissSizeToast = null;
        }
        return !text;
      };
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
      if (form.validateSelectedFiles && !form.validateSelectedFiles()) {
        resultCard.hidden = true;
        return;
      }

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
          showToast(payload.message || "创建分享失败");
          resultCard.hidden = true;
          return;
        }

        const shareURL = payload.data.shareUrl;
        shareUrlInput.value = shareURL;
        visitLink.href = shareURL;
        form.hidden = true;
        resultCard.hidden = false;
        resultCard.scrollIntoView({ behavior: "smooth", block: "nearest" });
      } catch (error) {
        if (form.updateUploadProgress) form.updateUploadProgress(0, 1);
        if (error && error.name === "AbortError") {
          showToast("已取消上传", "info");
        } else if (error && error.name === "NetworkError") {
          showToast("网络连接中断，请检查网络后重试");
        } else {
          showToast("上传中断，请确认文件未被移动、覆盖保存或同步修改，然后重新选择文件再试");
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
    });
  }

  async function copyText(text, trigger) {
    try {
      await navigator.clipboard.writeText(text);
      if (!trigger) return;
      const oldHTML = trigger.innerHTML;
      if (trigger.classList.contains("button-icon")) {
        trigger.innerHTML = '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"></path></svg>';
      } else {
        trigger.textContent = "已复制";
      }
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

  function bindSharingControls() {
    const dialog = $("[data-share-settings-dialog]");
    if (!dialog) return;
    const form = $("[data-reshare-form]", dialog);
    const result = $("[data-reshare-result]", dialog);
    const message = $("[data-share-settings-message]", dialog);
    const submit = $("button[type=submit]", form);
    const closeButtons = $$("[data-share-settings-close]", dialog);
    let saving = false;

    const updateSharingItem = (id, summary) => {
      const item = $$("[data-share-id]", dialog.closest("main")).find((entry) => entry.dataset.shareId === id);
      if (!item) return;
      const active = summary.status === "active";
      const action = $("[data-share-action]", item);
      action.dataset.shareAction = active ? "cancel" : "share";
      action.title = active ? "取消分享" : "分享文件";
      action.setAttribute("aria-label", action.title);
      action.innerHTML = active
        ? '<svg aria-hidden="true" width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 17H7A5 5 0 0 1 7 7"></path><path d="M15 7h2a5 5 0 0 1 4 8"></path><path d="M8 12h4M3 3l18 18"></path></svg>'
        : '<svg aria-hidden="true" width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 17H7A5 5 0 0 1 7 7h2M15 7h2a5 5 0 0 1 0 10h-2"></path><path d="M8 12h8"></path></svg>';
      const status = $(".badge-active, .badge-expired, .badge-cancelled", item);
      status.className = `badge badge-${summary.status}`;
      status.textContent = active ? "分享中" : "已取消";
      const copy = $("[data-copy-value]", item);
      copy.hidden = !active;
      copy.dataset.copyValue = active ? summary.shareUrl : "";
      if (!active) return;

      const badges = $(".share-item__badges", item);
      let passwordBadge = $(".badge-subtle", badges);
      if (summary.hasPassword && !passwordBadge) {
        passwordBadge = document.createElement("span");
        passwordBadge.className = "badge badge-subtle";
        passwordBadge.textContent = "有提取码";
        badges.prepend(passwordBadge);
      } else if (!summary.hasPassword && passwordBadge) {
        passwordBadge.remove();
      }
      const expiresAt = new Date(summary.expiresAt);
      const pad = (value) => String(value).padStart(2, "0");
      $("[data-share-expiry-time]", item).textContent = expiresAt.getUTCFullYear() >= 9999
        ? "过期：永不过期"
        : `过期：${expiresAt.getFullYear()}-${pad(expiresAt.getMonth() + 1)}-${pad(expiresAt.getDate())} ${pad(expiresAt.getHours())}:${pad(expiresAt.getMinutes())}`;
      const description = $("[data-description-form] textarea", item);
      description.value = summary.description;
      description.defaultValue = summary.description;
      setMessage($("[data-description-message]", item), "", "");
    };

    closeButtons.forEach((button) => button.addEventListener("click", () => {
      if (!saving) dialog.close();
    }));
    dialog.addEventListener("cancel", (event) => {
      if (saving) event.preventDefault();
    });

    $$("[data-share-action]").forEach((button) => {
      button.addEventListener("click", async () => {
        const id = button.dataset.sharingId;
        button.disabled = true;
        try {
          if (button.dataset.shareAction === "cancel") {
            const response = await fetch(`/api/admin/shares/${id}/cancel`, { method: "POST" });
            if (response.redirected) { window.location.href = response.url; return; }
            const payload = await response.json();
            if (!response.ok || !payload.success) {
              showNoticeDialog(payload.message || "取消分享失败");
              return;
            }
            updateSharingItem(id, { status: "cancelled" });
            return;
          }

          dialog.dataset.shareId = id;
          form.hidden = true;
          result.hidden = true;
          setMessage(message, "正在读取分享设置…", "");
          dialog.showModal();
          const response = await fetch(`/api/admin/shares/${id}/settings`, { cache: "no-store" });
          if (response.redirected) { window.location.href = response.url; return; }
          const payload = await response.json();
          if (!dialog.open || dialog.dataset.shareId !== id) return;
          if (!response.ok || !payload.success) {
            setMessage(message, payload.message || "读取设置失败，请关闭后重试", "error");
            return;
          }
          form.elements.description.value = payload.description;
          form.elements.access_password.value = payload.accessPassword;
          const hours = String(payload.expiresHours);
          form.elements.expires_hours.value = ["0", "1", "6", "24", "72", "168"].includes(hours) ? hours : "24";
          setMessage(message, "", "");
          form.hidden = false;
          form.elements.description.focus();
        } catch (error) {
          if (dialog.open) setMessage(message, "网络异常，请关闭后重试", "error");
          else showNoticeDialog("网络异常，请稍后重试");
        } finally {
          button.disabled = false;
        }
      });
    });

    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      if (saving) return;
      saving = true;
      submit.disabled = true;
      closeButtons.forEach((button) => { button.disabled = true; });
      submit.textContent = "生成中…";
      setMessage(message, "", "");
      try {
        const response = await fetch(`/api/admin/shares/${dialog.dataset.shareId}/reshare`, {
          method: "POST", body: new FormData(form),
        });
        if (response.redirected) { window.location.href = response.url; return; }
        const payload = await response.json();
        if (!response.ok || !payload.success) {
          setMessage(message, payload.message || "重新分享失败", "error");
          return;
        }
        updateSharingItem(dialog.dataset.shareId, payload.data);
        form.hidden = true;
        result.hidden = false;
        $("#reshare-url", dialog).value = payload.data.shareUrl;
        $("[data-reshare-visit]", dialog).href = payload.data.shareUrl;
      } catch (error) {
        setMessage(message, "网络异常，请稍后重试", "error");
      } finally {
        saving = false;
        submit.disabled = false;
        closeButtons.forEach((button) => { button.disabled = false; });
        submit.textContent = "生成新分享链接";
      }
    });
  }

  function bindFileUpdates() {
    const dialog = $("[data-file-update-dialog]");
    if (!dialog) return;
    const form = $("[data-file-update-form]", dialog);
    const input = $("input[type=file]", form);
    const submit = $("button[type=submit]", form);
    const close = $("[data-file-update-close]", dialog);
    const message = $("[data-file-update-message]", dialog);
    const progress = $("[data-file-update-progress]", dialog);
    let target = null;
    let uploading = false;
    let updated = false;
    const finish = () => {
      if (uploading) return;
      dialog.close();
      if (updated) window.location.reload();
    };
    close.addEventListener("click", finish);
    dialog.addEventListener("cancel", (event) => {
      // File inputs also emit a bubbling cancel event when the picker closes.
      if (event.target !== dialog) return;
      event.preventDefault();
      finish();
    });
    $$("[data-replace-file]").forEach((button) => {
      button.addEventListener("click", () => {
        target = button;
        updated = false;
        form.reset();
        input.disabled = false;
        submit.hidden = false;
        submit.disabled = false;
        submit.textContent = "上传并更新";
        close.textContent = "取消";
        progress.hidden = true;
        setMessage(message, "", "");
        $("[data-file-update-name]", dialog).textContent = `当前文件：${button.dataset.replaceName}`;
        dialog.showModal();
      });
    });
    form.addEventListener("submit", (event) => {
      event.preventDefault();
      if (uploading || updated || !target || !input.files.length) return;
      const file = input.files[0];
      const limit = Number(dialog.dataset.maxUploadSize);
      if (limit > 0 && file.size > limit) {
        setMessage(message, "文件大小超过上传上限，请重新选择", "error");
        return;
      }
      const body = new FormData();
      body.append("file", file);
      const request = new XMLHttpRequest();
      request.open("POST", `/api/admin/shares/${encodeURIComponent(target.dataset.replaceShare)}/files/${encodeURIComponent(target.dataset.replaceFile)}/replace`);
      uploading = true;
      input.disabled = true;
      submit.disabled = true;
      close.disabled = true;
      progress.value = 0;
      progress.hidden = false;
      submit.textContent = "上传中…";
      const started = Date.now();
      request.upload.addEventListener("progress", (e) => {
        if (!e.lengthComputable) return;
        const percent = Math.min(100, Math.round(e.loaded / e.total * 100));
        progress.value = percent;
        const speed = e.loaded / Math.max((Date.now() - started) / 1000, 0.1);
        const remaining = Math.max(0, Math.ceil((e.total - e.loaded) / speed));
        setMessage(message, percent === 100 ? "上传完成，正在保存…" : `上传 ${percent}% · ${(speed / 1024 / 1024).toFixed(1)} MB/s · 剩余约 ${remaining} 秒`, "info");
      });
      request.addEventListener("loadend", () => {
        uploading = false;
        close.disabled = false;
        input.disabled = false;
        submit.disabled = false;
        submit.textContent = "上传并更新";
        let payload = null;
        try { payload = JSON.parse(request.responseText); } catch (error) {}
        if (request.status === 200 && payload && payload.success) {
          updated = true;
          input.disabled = true;
          submit.hidden = true;
          close.textContent = "完成";
          setMessage(message, payload.message, "success");
        } else {
          setMessage(message, (payload && payload.message) || "未能确认更新结果，请刷新管理页检查后重试", "error");
        }
      });
      request.send(body);
    });
  }

  function bindDescriptionForms() {
    $$("[data-description-form]").forEach((form) => {
      const input = $("textarea", form);
      const button = $("button[type=submit]", form);
      const message = $("[data-description-message]", form);
      input.addEventListener("input", () => setMessage(message, "", ""));
      form.addEventListener("submit", async (event) => {
        event.preventDefault();
        if (button.disabled) return;
        button.disabled = true;
        input.disabled = true;
        button.textContent = "保存中…";
        setMessage(message, "", "");
        try {
          const response = await fetch(`/api/admin/shares/${form.dataset.descriptionForm}/description`, {
            method: "POST",
            body: new URLSearchParams({ description: input.value }),
          });
          if (response.redirected) {
            window.location.href = response.url;
            return;
          }
          const payload = await response.json();
          if (!response.ok || !payload.success) {
            setMessage(message, payload.message || "保存说明失败", "error");
            return;
          }
          input.value = payload.description;
          input.defaultValue = payload.description;
          setMessage(message, "已保存", "success");
        } catch (error) {
          setMessage(message, "网络异常，请稍后重试", "error");
        } finally {
          button.disabled = false;
          input.disabled = false;
          button.textContent = "保存说明";
        }
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
      $$("[data-admin-download], [data-replace-file]", deletedItem).forEach((link) => link.remove());
      const editor = $(".share-description-editor", deletedItem);
      if (editor) {
        const input = $("textarea", editor);
        if (input && input.defaultValue) {
          const text = document.createElement("p");
          text.className = "share-description-text";
          text.textContent = input.defaultValue;
          $("summary", editor).textContent = "查看说明";
          $("form", editor).replaceWith(text);
        } else {
          editor.remove();
        }
      }
      let actions = $(".share-item__actions", deletedItem);
      if (actions) {
        $$("[data-copy-value], [data-delete-share], [data-share-action]", actions).forEach((control) => control.remove());
      } else {
        actions = document.createElement("div");
        actions.className = "share-item__actions";
        const main = $(".share-item__main", deletedItem);
        if (main) main.insertAdjacentElement("afterend", actions);
      }
      const purgeButton = createPurgeButton(shareId);
      purgeButton.addEventListener("click", () => purgeDeletedShare(purgeButton));
      actions.append(purgeButton);
      const status = $(".badge-active, .badge-expired, .badge-cancelled", deletedItem);
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
        toggleButton.setAttribute("aria-label", "展开文件列表");
        toggleButton.title = "展开文件列表";
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
    modal.addEventListener("keydown", (event) => {
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
    const label = expanded ? "展开文件列表" : "收起文件列表";
    button.setAttribute("aria-label", label);
    button.title = label;
  }

  function bindCurrentView(main) {
    if (!main || main.dataset.viewBound === "true") return;
    main.dataset.viewBound = "true";

    if ($("#login-form", main)) bindLoginForm();
    if ($("#share-form", main)) {
      bindDropzone();
      bindCustomSelect();
      bindUploadForm();
    }
    if ($("[data-management-tab]", main)) {
      bindSharingControls();
      bindFileUpdates();
      bindDescriptionForms();
      bindDeleteButtons();
      bindFileDisclosures();
      bindManagementTabs();
    }

    bindCopyButtons();
    bindDownloadCounters();
  }

  function bindAppNavigation() {
    const shell = $(".app-shell");
    const links = $$('[data-app-view]');
    if (!shell || links.length === 0) return;

    const routes = {
      upload: "/upload",
      admin: "/admin/shares",
      about: "/about",
    };
    const initialMain = $(".app-shell > main");
    const viewForPath = () => {
      if (window.location.pathname === routes.admin) return "admin";
      if (window.location.pathname === routes.about) return "about";
      return "upload";
    };
    const initialView = viewForPath();
    const uploadView = {
      main: initialView === "upload" ? initialMain : null,
      title: initialView === "upload" ? document.title : "上传文件 - MgFileBox",
      scrollY: 0,
    };
    let currentView = initialView;
    let switching = false;

    const updateNavigation = (view) => {
      $$(".nav-pill[data-app-view]").forEach((link) => {
        link.classList.toggle("is-active", link.dataset.appView === view);
      });
    };

    const loadView = async (view) => {
      const response = await fetch(routes[view]);
      const responsePath = new URL(response.url, window.location.href).pathname;
      if (!response.ok || responsePath === "/login") {
        window.location.href = responsePath === "/login" ? "/login" : routes[view];
        return null;
      }

      const page = new DOMParser().parseFromString(await response.text(), "text/html");
      const main = $(".app-shell > main", page);
      if (!main) {
        window.location.href = routes[view];
        return null;
      }

      return {
        main,
        modal: $("[data-delete-modal]", page),
        title: page.title,
      };
    };

    const switchView = async (view, pushHistory) => {
      if (!routes[view] || view === currentView || switching) return;
      switching = true;

      try {
        const currentMain = $(".app-shell > main");
        let next = null;

        if (view === "upload" && uploadView.main) {
          next = uploadView;
        } else {
          next = await loadView(view);
          if (!next) return;
          if (view === "upload") {
            uploadView.main = next.main;
            uploadView.title = next.title;
          }
        }

        if (currentView === "upload") uploadView.scrollY = window.scrollY;
        currentMain.replaceWith(next.main);

        const oldModal = $("[data-delete-modal]");
        if (oldModal) oldModal.remove();
        if (view === "admin" && next.modal) document.body.append(next.modal);

        currentView = view;
        document.title = next.title;
        updateNavigation(view);
        bindCurrentView(next.main);

        if (pushHistory) history.pushState({ mgFileBoxView: view }, "", routes[view]);
        window.scrollTo(0, view === "upload" ? uploadView.scrollY : 0);
      } catch (error) {
        window.location.href = routes[view];
      } finally {
        switching = false;
      }
    };

    links.forEach((link) => {
      link.addEventListener("click", (event) => {
        if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
        event.preventDefault();
        switchView(link.dataset.appView, true);
      });
    });

    window.addEventListener("popstate", () => {
      switchView(viewForPath(), false);
    });
  }

  bindCurrentView($("main"));
  bindAppNavigation();
})();
