// Telesend Web Console JS
(function () {
  'use strict';

  // State
  let botsList = [];
  let aliasesList = [];
  let historyList = [];
  let historyRetentionDays = 7;
  let currentAliasFilter = '';
  let serverInfo = null;

  // DOM Elements
  const appVersionEl = document.getElementById('app-version');
  const globalAlert = document.getElementById('global-alert');
  const globalAlertText = document.getElementById('global-alert-text');
  const themeToggle = document.getElementById('theme-toggle');
  const menuBtn = document.getElementById('menu-btn');
  const accountMenu = document.getElementById('account-menu');
  const logoutBtn = document.getElementById('logout-btn');

  // Modals
  const modalLogin = document.getElementById('modal-login');
  const modalAddBot = document.getElementById('modal-add-bot');
  const modalEditBot = document.getElementById('modal-edit-bot');
  const modalAddAlias = document.getElementById('modal-add-alias');
  const modalEditAlias = document.getElementById('modal-edit-alias');
  const modalHistoryDetail = document.getElementById('modal-history-detail');
  const modalConfirm = document.getElementById('modal-confirm');
  const modalDeleteHistory = document.getElementById('modal-delete-history');
  const modalClearHistory = document.getElementById('modal-clear-history');

  // Reusable lightweight SVG icons
  const ICONS = {
    edit: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>',
    delete: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/></svg>',
    detail: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>',
  };

  // Centralized Confirmation Modal
  const confirmModalTitle = document.getElementById('confirm-modal-title');
  const confirmModalPrompt = document.getElementById('confirm-modal-prompt');
  const confirmModalError = document.getElementById('confirm-modal-error');
  const submitConfirmBtn = document.getElementById('submit-confirm-modal');
  const closeConfirmBtn = document.getElementById('close-confirm-modal');
  const cancelConfirmBtn = document.getElementById('cancel-confirm-modal');
  const formConfirmModal = document.getElementById('form-confirm-modal');
  let confirmActionHandler = null;

  function showConfirmDialog({ title = 'Confirm Action', prompt = 'Are you sure?', confirmText = 'Delete', danger = true, onConfirm }) {
    if (confirmModalTitle) confirmModalTitle.textContent = title;
    if (confirmModalPrompt) confirmModalPrompt.textContent = prompt;
    if (submitConfirmBtn) {
      submitConfirmBtn.textContent = confirmText;
      submitConfirmBtn.className = danger ? 'btn btn-danger' : 'btn btn-primary';
    }
    if (confirmModalError) {
      confirmModalError.textContent = '';
      confirmModalError.classList.add('hidden');
    }
    confirmActionHandler = onConfirm;
    if (modalConfirm && typeof modalConfirm.showModal === 'function') {
      modalConfirm.showModal();
    }
  }

  if (closeConfirmBtn) closeConfirmBtn.addEventListener('click', () => modalConfirm.close());
  if (cancelConfirmBtn) cancelConfirmBtn.addEventListener('click', () => modalConfirm.close());
  if (formConfirmModal) {
    formConfirmModal.addEventListener('submit', async (e) => {
      e.preventDefault();
      if (!confirmActionHandler) {
        modalConfirm.close();
        return;
      }
      try {
        await confirmActionHandler();
        modalConfirm.close();
      } catch (err) {
        if (confirmModalError) {
          confirmModalError.textContent = err.message || 'Operation failed';
          confirmModalError.classList.remove('hidden');
        }
      }
    });
  }

  // Tabs
  const navTabs = document.querySelectorAll('.nav-item');
  const tabPanels = document.querySelectorAll('.tab-panel');

  // Initialize Theme
  function initTheme() {
    const saved = localStorage.getItem('telesend_theme');
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    const theme = saved || (prefersDark ? 'dark' : 'light');
    document.documentElement.setAttribute('data-theme', theme);
  }

  themeToggle.addEventListener('click', () => {
    const current = document.documentElement.getAttribute('data-theme') || 'dark';
    const next = current === 'dark' ? 'light' : 'dark';
    document.documentElement.setAttribute('data-theme', next);
    localStorage.setItem('telesend_theme', next);
  });

  // Alerts
  function showAlert(msg, isError = false) {
    globalAlert.classList.remove('hidden', 'alert-success', 'alert-error');
    globalAlert.classList.add(isError ? 'alert-error' : 'alert-success');
    globalAlertText.textContent = msg;
    setTimeout(() => {
      globalAlert.classList.add('hidden');
    }, 5000);
  }

  globalAlert.querySelector('.alert-close').addEventListener('click', () => {
    globalAlert.classList.add('hidden');
  });

  // Account Menu Toggle
  menuBtn.addEventListener('click', (e) => {
    e.stopPropagation();
    const open = !accountMenu.classList.contains('hidden');
    accountMenu.classList.toggle('hidden', open);
    menuBtn.setAttribute('aria-expanded', String(!open));
  });

  document.addEventListener('click', (e) => {
    if (!accountMenu.contains(e.target) && e.target !== menuBtn) {
      accountMenu.classList.add('hidden');
      menuBtn.setAttribute('aria-expanded', 'false');
    }
  });

  // Password Visibility Toggle
  document.querySelectorAll('.btn-toggle-mask').forEach((btn) => {
    btn.addEventListener('click', () => {
      const targetId = btn.getAttribute('data-target');
      const input = document.getElementById(targetId);
      if (!input) return;
      const isPw = input.type === 'password';
      input.type = isPw ? 'text' : 'password';
      btn.textContent = isPw ? 'hide' : 'show';
    });
  });

  // Tab Navigation
  navTabs.forEach((tab) => {
    tab.addEventListener('click', () => {
      const target = tab.getAttribute('data-tab');
      navTabs.forEach((t) => {
        t.classList.remove('active');
        t.setAttribute('aria-selected', 'false');
      });
      tabPanels.forEach((p) => p.classList.remove('active'));

      tab.classList.add('active');
      tab.setAttribute('aria-selected', 'true');
      const targetPanel = document.getElementById(`panel-${target}`);
      if (targetPanel) targetPanel.classList.add('active');

      if (target === 'history') {
        loadHistory();
      }
    });
  });

  // API Client Helper
  async function apiCall(endpoint, options = {}) {
    try {
      const res = await fetch(`/api/ui${endpoint}`, {
        headers: {
          'X-Requested-With': 'XMLHttpRequest',
          ...(options.headers || {}),
        },
        ...options,
      });

      if (res.status === 401 && !endpoint.startsWith('/auth/')) {
        // Session expired or unauthenticated
        openLoginModal();
        throw new Error('Session expired. Please log in.');
      }

      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        throw new Error(data.error || `Request failed with code ${res.status}`);
      }
      return data;
    } catch (err) {
      throw err;
    }
  }

  // Auth Handling
  async function checkAuth() {
    try {
      const status = await apiCall('/auth/status');
      if (status.version && appVersionEl) {
        appVersionEl.textContent = `v${status.version}`;
      }

      if (!status.authenticated) {
        openLoginModal();
        return;
      }

      // Authenticated
      closeAllModals();
      loadDashboard();
    } catch (err) {
      showAlert(`Initialization error: ${err.message}`, true);
    }
  }

  function openLoginModal() {
    closeAllModals();
    if (typeof modalLogin.showModal === 'function') {
      modalLogin.showModal();
    }
  }

  function closeAllModals() {
    [modalLogin, modalAddBot, modalEditBot, modalAddAlias, modalEditAlias, modalHistoryDetail, modalConfirm, modalDeleteHistory, modalClearHistory].forEach((m) => {
      if (m && m.open && typeof m.close === 'function') m.close();
    });
  }

  // Login Form
  document.getElementById('form-login').addEventListener('submit', async (e) => {
    e.preventDefault();
    const pw = document.getElementById('login-pw').value;
    const errBox = document.getElementById('login-error');
    errBox.classList.add('hidden');

    try {
      await apiCall('/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password: pw }),
      });
      document.getElementById('login-pw').value = '';
      modalLogin.close();
      loadDashboard();
    } catch (err) {
      errBox.textContent = err.message;
      errBox.classList.remove('hidden');
    }
  });

  // Logout
  logoutBtn.addEventListener('click', async () => {
    accountMenu.classList.add('hidden');
    try {
      await apiCall('/auth/logout', { method: 'POST' });
    } catch (_) {}
    openLoginModal();
  });

  // Dashboard Data Loading
  async function loadDashboard() {
    await Promise.allSettled([loadBots(), loadAliases(), loadHistory(), loadServerInfo()]);
    updateRecipientDropdown();
  }

  // BOTS MANAGEMENT
  async function loadBots() {
    const listEl = document.getElementById('bot-list');
    try {
      const res = await apiCall('/bots');
      botsList = res.data || [];
      document.getElementById('tab-bot-count').textContent = String(botsList.length);
      renderBots();
      updateBotDropdown();
    } catch (err) {
      listEl.innerHTML = `<div class="empty-state text-danger">Failed to load bots: ${escapeHtml(err.message)}</div>`;
    }
  }

  function renderBots() {
    const listEl = document.getElementById('bot-list');
    if (!botsList || botsList.length === 0) {
      listEl.innerHTML = `
        <div class="empty-state">
          No bots registered yet.<br>Click "+ Add Bot" to register a Telegram bot token.
        </div>`;
      return;
    }

    listEl.innerHTML = botsList.map((bot) => `
      <div class="item-row" data-username="${escapeHtml(bot.username)}">
        <div class="item-main">
          <div class="item-title-row">
            <span class="item-title code-font">@${escapeHtml(bot.username)}</span>
            <span class="text-muted text-xs">${escapeHtml(bot.name)}</span>
            ${bot.isDefault ? '<span class="badge badge-success">DEFAULT</span>' : ''}
          </div>
          <div class="item-subtitle code-font">
            Telegram ID: ${escapeHtml(bot.telegramId)}
          </div>
        </div>
        <div class="item-actions">
          ${!bot.isDefault ? `
            <button type="button" class="btn btn-sm btn-subtle btn-set-default" data-username="${escapeHtml(bot.username)}" title="Set as default bot">
              Set default
            </button>` : ''}
          <button type="button" class="btn btn-sm btn-subtle btn-action-icon btn-edit-bot" data-username="${escapeHtml(bot.username)}" data-name="${escapeHtml(bot.name)}" title="Edit bot" aria-label="Edit bot">
            ${ICONS.edit}
          </button>
          <button type="button" class="btn btn-sm btn-danger btn-action-icon btn-remove-bot" data-username="${escapeHtml(bot.username)}" title="Delete bot" aria-label="Delete bot">
            ${ICONS.delete}
          </button>
        </div>
      </div>
    `).join('');

    // Attach row events
    listEl.querySelectorAll('.btn-set-default').forEach((btn) => {
      btn.addEventListener('click', async () => {
        const username = btn.getAttribute('data-username');
        try {
          await apiCall(`/bots/${encodeURIComponent(username)}/default`, { method: 'POST' });
          showAlert(`Default bot switched to @${username}.`);
          loadBots();
        } catch (err) {
          showAlert(err.message, true);
        }
      });
    });

    listEl.querySelectorAll('.btn-edit-bot').forEach((btn) => {
      btn.addEventListener('click', () => {
        const username = btn.getAttribute('data-username');
        const name = btn.getAttribute('data-name');
        document.getElementById('edit-bot-username').value = username;
        document.getElementById('edit-bot-username-display').textContent = `@${username}`;
        document.getElementById('edit-bot-name').value = name;
        document.getElementById('edit-bot-token').value = '';
        document.getElementById('edit-bot-error').classList.add('hidden');
        modalEditBot.showModal();
      });
    });

    listEl.querySelectorAll('.btn-remove-bot').forEach((btn) => {
      btn.addEventListener('click', () => {
        const username = btn.getAttribute('data-username');
        showConfirmDialog({
          title: 'Delete Bot',
          prompt: `Are you sure you want to remove bot @${username}?`,
          confirmText: 'Delete',
          danger: true,
          onConfirm: async () => {
            await apiCall(`/bots/${encodeURIComponent(username)}`, { method: 'DELETE' });
            showAlert(`Bot @${username} removed.`);
            loadBots();
          },
        });
      });
    });
  }

  // Add Bot Modal Form
  const toggleAddBotBtn = document.getElementById('toggle-add-bot-btn');
  const closeAddBotBtn = document.getElementById('close-add-bot');
  const cancelAddBotBtn = document.getElementById('cancel-add-bot');
  const formAddBot = document.getElementById('form-add-bot');
  const addBotError = document.getElementById('add-bot-error');

  if (toggleAddBotBtn) {
    toggleAddBotBtn.addEventListener('click', () => {
      if (formAddBot) formAddBot.reset();
      if (addBotError) addBotError.classList.add('hidden');
      if (modalAddBot && typeof modalAddBot.showModal === 'function') {
        modalAddBot.showModal();
        const inp = document.getElementById('bot-token-input');
        if (inp) inp.focus();
      }
    });
  }
  if (closeAddBotBtn) closeAddBotBtn.addEventListener('click', () => modalAddBot.close());
  if (cancelAddBotBtn) cancelAddBotBtn.addEventListener('click', () => modalAddBot.close());

  if (formAddBot) {
    formAddBot.addEventListener('submit', async (e) => {
      e.preventDefault();
      const token = document.getElementById('bot-token-input').value.trim();
      if (!token) return;
      if (addBotError) addBotError.classList.add('hidden');

      try {
        const res = await apiCall('/bots', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ token }),
        });
        formAddBot.reset();
        modalAddBot.close();
        showAlert(`Bot @${res.data.username} registered.`);
        loadBots();
      } catch (err) {
        if (addBotError) {
          addBotError.textContent = err.message;
          addBotError.classList.remove('hidden');
        } else {
          showAlert(err.message, true);
        }
      }
    });
  }

  // Edit Bot Form
  document.getElementById('close-edit-bot').addEventListener('click', () => modalEditBot.close());
  document.getElementById('cancel-edit-bot').addEventListener('click', () => modalEditBot.close());

  document.getElementById('form-edit-bot').addEventListener('submit', async (e) => {
    e.preventDefault();
    const username = document.getElementById('edit-bot-username').value;
    const name = document.getElementById('edit-bot-name').value.trim();
    const token = document.getElementById('edit-bot-token').value.trim();
    const errBox = document.getElementById('edit-bot-error');
    errBox.classList.add('hidden');

    try {
      await apiCall(`/bots/${encodeURIComponent(username)}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, token }),
      });
      modalEditBot.close();
      showAlert(`Bot @${username} updated.`);
      loadBots();
    } catch (err) {
      errBox.textContent = err.message;
      errBox.classList.remove('hidden');
    }
  });

  // ALIASES MANAGEMENT
  async function loadAliases() {
    const listEl = document.getElementById('alias-list');
    try {
      const q = currentAliasFilter ? `?filter=${encodeURIComponent(currentAliasFilter)}` : '';
      const res = await apiCall(`/aliases${q}`);
      aliasesList = res.data || [];
      document.getElementById('tab-alias-count').textContent = String(aliasesList.length);
      document.getElementById('alias-filter-count').textContent = `${aliasesList.length} aliases`;
      renderAliases();
      updateRecipientDropdown();
    } catch (err) {
      listEl.innerHTML = `<div class="empty-state text-danger">Failed to load aliases: ${escapeHtml(err.message)}</div>`;
    }
  }

  function renderAliases() {
    const listEl = document.getElementById('alias-list');
    if (!aliasesList || aliasesList.length === 0) {
      listEl.innerHTML = `
        <div class="empty-state">
          No aliases found.${currentAliasFilter ? ' (Try changing filter)' : '<br>Click "+ Add Alias" to create one.'}
        </div>`;
      return;
    }

    listEl.innerHTML = aliasesList.map((al) => `
      <div class="item-row" data-name="${escapeHtml(al.name)}">
        <div class="item-main">
          <div class="item-title-row">
            <span class="item-title">${escapeHtml(al.name)}</span>
            <span class="badge ${al.type === 'group' ? 'badge-get' : 'badge-subtle'}">${escapeHtml(al.type)}</span>
            ${al.messageThreadId ? `<span class="badge badge-subtle">topic #${al.messageThreadId}</span>` : ''}
          </div>
          <div class="item-subtitle code-font">
            Chat ID: ${escapeHtml(al.chatId)}
          </div>
        </div>
        <div class="item-actions">
          <button type="button" class="btn btn-sm btn-subtle btn-quick-send" data-name="${escapeHtml(al.name)}" title="Send message to this alias">
            Send
          </button>
          <button type="button" class="btn btn-sm btn-subtle btn-action-icon btn-edit-alias"
            data-name="${escapeHtml(al.name)}"
            data-chat-id="${escapeHtml(al.chatId)}"
            data-thread="${al.messageThreadId || ''}"
            title="Edit alias" aria-label="Edit alias">
            ${ICONS.edit}
          </button>
          <button type="button" class="btn btn-sm btn-danger btn-action-icon btn-remove-alias" data-name="${escapeHtml(al.name)}" title="Delete alias" aria-label="Delete alias">
            ${ICONS.delete}
          </button>
        </div>
      </div>
    `).join('');

    // Attach row events
    listEl.querySelectorAll('.btn-quick-send').forEach((btn) => {
      btn.addEventListener('click', () => {
        const name = btn.getAttribute('data-name');
        // Switch to send tab
        document.querySelector('.nav-item[data-tab="send"]').click();
        const sel = document.getElementById('send-recipient-select');
        sel.value = name;
        sel.dispatchEvent(new Event('change'));
        document.getElementById('send-text-content').focus();
      });
    });

    listEl.querySelectorAll('.btn-edit-alias').forEach((btn) => {
      btn.addEventListener('click', () => {
        const name = btn.getAttribute('data-name');
        const chatId = btn.getAttribute('data-chat-id');
        const thread = btn.getAttribute('data-thread');
        document.getElementById('edit-alias-old-name').value = name;
        document.getElementById('edit-alias-name').value = name;
        document.getElementById('edit-alias-chat-id').value = chatId;
        document.getElementById('edit-alias-thread-id').value = thread;
        document.getElementById('edit-alias-error').classList.add('hidden');
        modalEditAlias.showModal();
      });
    });

    listEl.querySelectorAll('.btn-remove-alias').forEach((btn) => {
      btn.addEventListener('click', () => {
        const name = btn.getAttribute('data-name');
        showConfirmDialog({
          title: 'Delete Alias',
          prompt: `Are you sure you want to delete alias "${name}"?`,
          confirmText: 'Delete',
          danger: true,
          onConfirm: async () => {
            await apiCall(`/aliases/${encodeURIComponent(name)}`, { method: 'DELETE' });
            showAlert(`Alias "${name}" deleted.`);
            loadAliases();
          },
        });
      });
    });
  }

  // Filter Pills
  document.querySelectorAll('.filter-pill').forEach((pill) => {
    pill.addEventListener('click', () => {
      document.querySelectorAll('.filter-pill').forEach((p) => p.classList.remove('active'));
      pill.classList.add('active');
      currentAliasFilter = pill.getAttribute('data-filter') || '';
      loadAliases();
    });
  });

  // Add Alias Modal Form
  const toggleAddAliasBtn = document.getElementById('toggle-add-alias-btn');
  const closeAddAliasBtn = document.getElementById('close-add-alias');
  const cancelAddAliasBtn = document.getElementById('cancel-add-alias');
  const formAddAlias = document.getElementById('form-add-alias');
  const addAliasError = document.getElementById('add-alias-error');

  if (toggleAddAliasBtn) {
    toggleAddAliasBtn.addEventListener('click', () => {
      if (formAddAlias) formAddAlias.reset();
      if (addAliasError) addAliasError.classList.add('hidden');
      if (modalAddAlias && typeof modalAddAlias.showModal === 'function') {
        modalAddAlias.showModal();
        const inp = document.getElementById('alias-name-input');
        if (inp) inp.focus();
      }
    });
  }
  if (closeAddAliasBtn) closeAddAliasBtn.addEventListener('click', () => modalAddAlias.close());
  if (cancelAddAliasBtn) cancelAddAliasBtn.addEventListener('click', () => modalAddAlias.close());

  if (formAddAlias) {
    formAddAlias.addEventListener('submit', async (e) => {
      e.preventDefault();
      const name = document.getElementById('alias-name-input').value.trim();
      const chatId = document.getElementById('alias-chat-id-input').value.trim();
      const threadStr = document.getElementById('alias-thread-id-input').value.trim();
      const messageThreadId = threadStr ? parseInt(threadStr, 10) : null;
      if (addAliasError) addAliasError.classList.add('hidden');

      try {
        await apiCall('/aliases', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name, chatId, messageThreadId }),
        });
        formAddAlias.reset();
        modalAddAlias.close();
        showAlert(`Alias "${name}" created.`);
        loadAliases();
      } catch (err) {
        if (addAliasError) {
          addAliasError.textContent = err.message;
          addAliasError.classList.remove('hidden');
        } else {
          showAlert(err.message, true);
        }
      }
    });
  }

  // Edit Alias Form
  document.getElementById('close-edit-alias').addEventListener('click', () => modalEditAlias.close());
  document.getElementById('cancel-edit-alias').addEventListener('click', () => modalEditAlias.close());

  document.getElementById('form-edit-alias').addEventListener('submit', async (e) => {
    e.preventDefault();
    const oldName = document.getElementById('edit-alias-old-name').value;
    const name = document.getElementById('edit-alias-name').value.trim();
    const chatId = document.getElementById('edit-alias-chat-id').value.trim();
    const threadStr = document.getElementById('edit-alias-thread-id').value.trim();
    const messageThreadId = threadStr ? parseInt(threadStr, 10) : null;
    const errBox = document.getElementById('edit-alias-error');
    errBox.classList.add('hidden');

    try {
      await apiCall(`/aliases/${encodeURIComponent(oldName)}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, chatId, messageThreadId }),
      });
      modalEditAlias.close();
      showAlert(`Alias "${name}" updated.`);
      loadAliases();
    } catch (err) {
      errBox.textContent = err.message;
      errBox.classList.remove('hidden');
    }
  });

  // SEND MESSAGE HANDLING
  function updateBotDropdown() {
    const sel = document.getElementById('send-bot-select');
    sel.innerHTML = '<option value="">Default bot</option>';
    botsList.forEach((b) => {
      const opt = document.createElement('option');
      opt.value = b.username;
      opt.textContent = `@${b.username}${b.isDefault ? ' (Default)' : ''}`;
      sel.appendChild(opt);
    });
  }

  function updateRecipientDropdown() {
    const sel = document.getElementById('send-recipient-select');
    const curVal = sel.value;
    sel.innerHTML = '<option value="" disabled selected>Select alias or enter ID...</option>';

    aliasesList.forEach((al) => {
      const opt = document.createElement('option');
      opt.value = al.name;
      opt.textContent = `${al.name} (${al.type}${al.messageThreadId ? ` #${al.messageThreadId}` : ''})`;
      sel.appendChild(opt);
    });

    const customOpt = document.createElement('option');
    customOpt.value = '__custom__';
    customOpt.textContent = 'Custom Chat ID or topic...';
    sel.appendChild(customOpt);

    if (curVal && sel.querySelector(`option[value="${curVal}"]`)) {
      sel.value = curVal;
    }
  }

  const recipientSelect = document.getElementById('send-recipient-select');
  const customRecipientGroup = document.getElementById('custom-recipient-group');
  recipientSelect.addEventListener('change', () => {
    if (recipientSelect.value === '__custom__') {
      customRecipientGroup.classList.remove('hidden');
      document.getElementById('send-custom-recipient').focus();
    } else {
      customRecipientGroup.classList.add('hidden');
    }
  });

  // Message Type Selector
  const msgTypeSelect = document.getElementById('send-type-select');
  const typeSections = {
    text: document.getElementById('type-section-text'),
    photo: document.getElementById('type-section-media'),
    document: document.getElementById('type-section-media'),
    audio: document.getElementById('type-section-media'),
    video: document.getElementById('type-section-media'),
    animation: document.getElementById('type-section-media'),
    voice: document.getElementById('type-section-media'),
    location: document.getElementById('type-section-location'),
    poll: document.getElementById('type-section-poll'),
    dice: document.getElementById('type-section-dice'),
    raw: document.getElementById('type-section-raw'),
  };

  msgTypeSelect.addEventListener('change', () => {
    const val = msgTypeSelect.value;
    Object.keys(typeSections).forEach((k) => {
      if (typeSections[k]) typeSections[k].classList.add('hidden');
    });
    if (typeSections[val]) {
      typeSections[val].classList.remove('hidden');
    }
  });

  // Media Source Radio Toggle (Upload vs URL)
  let activeMediaSource = 'upload';
  const mediaSourceRadios = document.querySelectorAll('input[name="media-source"]');
  mediaSourceRadios.forEach((radio) => {
    radio.addEventListener('change', () => {
      if (radio.checked) {
        activeMediaSource = radio.value;
        const isUpload = activeMediaSource === 'upload';
        document.getElementById('media-upload-group').classList.toggle('hidden', !isUpload);
        document.getElementById('media-url-group').classList.toggle('hidden', isUpload);
      }
    });
  });

  // Send Message Submit
  document.getElementById('form-send-message').addEventListener('submit', async (e) => {
    e.preventDefault();
    const resultBox = document.getElementById('send-result-box');
    resultBox.classList.add('hidden');

    let to = recipientSelect.value;
    if (to === '__custom__') {
      to = document.getElementById('send-custom-recipient').value.trim();
    }
    if (!to) {
      showAlert('Recipient is required.', true);
      return;
    }

    const bot = document.getElementById('send-bot-select').value;
    const threadStr = document.getElementById('send-thread-override').value.trim();
    const threadId = threadStr ? parseInt(threadStr, 10) : 0;
    const typ = msgTypeSelect.value;

    const btn = document.getElementById('btn-submit-send');
    btn.disabled = true;
    btn.textContent = 'Sending...';

    try {
      let res;
      const checkedSource = document.querySelector('input[name="media-source"]:checked');
      if (checkedSource) activeMediaSource = checkedSource.value;
      const silent = document.getElementById('send-silent')?.checked || false;

      if (['photo', 'document', 'audio', 'video', 'animation', 'voice'].includes(typ) && activeMediaSource === 'upload') {
        // Multipart form-data
        const fileInput = document.getElementById('send-media-file');
        if (!fileInput.files || fileInput.files.length === 0) {
          throw new Error('Please select a file to upload.');
        }
        const fd = new FormData();
        fd.append('type', typ);
        fd.append('to', to);
        if (bot) fd.append('bot', bot);
        if (threadId > 0) fd.append('threadId', String(threadId));
        if (silent) fd.append('silent', 'true');
        const cap = document.getElementById('send-media-caption').value.trim();
        if (cap) fd.append('caption', cap);
        fd.append(typ, fileInput.files[0]);

        res = await apiCall('/send', {
          method: 'POST',
          body: fd,
        });
      } else {
        // JSON payload
        let payload = {};

        if (typ === 'text') {
          const text = document.getElementById('send-text-content').value;
          if (!text) throw new Error('Message text is required.');
          payload.text = text;
          const parseMode = document.getElementById('send-parse-mode').value;
          if (parseMode) payload.parse_mode = parseMode;
        } else if (['photo', 'document', 'audio', 'video', 'animation', 'voice'].includes(typ)) {
          const val = document.getElementById('send-media-url').value.trim();
          if (!val) throw new Error('URL or File ID is required.');
          let srcObj;
          if (val.startsWith('file_id:')) {
            srcObj = { source: 'file_id', value: val.slice('file_id:'.length) };
          } else if (val.startsWith('http://') || val.startsWith('https://')) {
            srcObj = { source: 'url', value: val };
          } else {
            srcObj = { source: 'file_id', value: val };
          }
          payload[typ] = srcObj;
          const cap = document.getElementById('send-media-caption').value.trim();
          if (cap) payload.caption = cap;
        } else if (typ === 'location') {
          const lat = parseFloat(document.getElementById('send-lat').value);
          const lon = parseFloat(document.getElementById('send-lon').value);
          if (isNaN(lat) || isNaN(lon)) throw new Error('Valid latitude and longitude required.');
          payload.latitude = lat;
          payload.longitude = lon;
        } else if (typ === 'poll') {
          const q = document.getElementById('send-poll-q').value.trim();
          const optsRaw = document.getElementById('send-poll-options').value.trim();
          if (!q) throw new Error('Poll question is required.');
          const options = optsRaw.split('\n').map((o) => o.trim()).filter(Boolean).map((t) => ({ text: t }));
          if (options.length < 2) throw new Error('Poll requires at least 2 options.');
          payload.question = q;
          payload.options = options;
        } else if (typ === 'dice') {
          payload.emoji = document.getElementById('send-dice-emoji').value;
        } else if (typ === 'raw') {
          const raw = document.getElementById('send-raw-json').value.trim();
          try {
            payload = JSON.parse(raw);
          } catch (_) {
            throw new Error('Invalid JSON payload.');
          }
        }

        res = await apiCall('/send', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            type: typ === 'raw' ? (payload.type || 'text') : typ,
            to,
            bot,
            threadId,
            silent,
            payload,
          }),
        });
      }

      const delivery = res.data;
      loadHistory();
      resultBox.className = 'delivery-result success';
      resultBox.innerHTML = `
        <div class="delivery-header">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>
          Message Delivered Successfully
        </div>
        <div class="delivery-meta">
          <div>Bot: <span class="code-font">@${escapeHtml(delivery.bot)}</span></div>
          <div>Chat ID: <span class="code-font">${escapeHtml(delivery.chatId)}</span></div>
          ${delivery.messageId ? `<div>Message ID: <span class="code-font">${delivery.messageId}</span></div>` : ''}
          ${delivery.messageThreadId ? `<div>Topic: <span class="code-font">#${delivery.messageThreadId}</span></div>` : ''}
        </div>
      `;
      resultBox.classList.remove('hidden');
    } catch (err) {
      resultBox.className = 'delivery-result error';
      resultBox.innerHTML = `
        <div class="delivery-header">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
          Delivery Failed
        </div>
        <div class="delivery-meta">${escapeHtml(err.message)}</div>
      `;
      resultBox.classList.remove('hidden');
    } finally {
      btn.disabled = false;
      btn.textContent = 'Send Message';
    }
  });

  // SERVER & MCP INFO
  async function loadServerInfo() {
    try {
      const res = await apiCall('/server-info');
      serverInfo = res;
      const verEl = document.getElementById('srv-version');
      if (verEl) verEl.textContent = `v${res.version}`;
      const botApiEl = document.getElementById('srv-bot-api');
      if (botApiEl) botApiEl.textContent = `Telegram Bot API ${res.botApiVersion}`;

      const restStatusEl = document.getElementById('srv-rest-status');
      if (restStatusEl) {
        if (res.restActive) {
          restStatusEl.innerHTML = '<span class="badge badge-success">Active</span>';
        } else {
          restStatusEl.innerHTML = '<span class="badge badge-danger">Inactive</span>';
        }
      }

      const mcpStatusEl = document.getElementById('srv-mcp-status');
      if (mcpStatusEl) {
        if (res.mcpActive) {
          mcpStatusEl.innerHTML = '<span class="badge badge-success">Active</span>';
        } else {
          mcpStatusEl.innerHTML = '<span class="badge badge-danger">Inactive</span>';
        }
      }
    } catch (err) {
      console.warn('Failed to load server info', err);
    }
  }

  // SERVER STATUS MODAL
  const modalServerStatus = document.getElementById('modal-server-status');
  const serverStatusBtn = document.getElementById('server-status-btn');
  const closeServerStatus = document.getElementById('close-server-status');
  const closeServerStatusBtn = document.getElementById('close-server-status-btn');

  if (serverStatusBtn && modalServerStatus) {
    serverStatusBtn.addEventListener('click', () => {
      const accMenu = document.getElementById('account-menu');
      if (accMenu) accMenu.classList.add('hidden');
      loadServerInfo();
      modalServerStatus.showModal();
    });
  }
  if (closeServerStatus && modalServerStatus) {
    closeServerStatus.addEventListener('click', () => modalServerStatus.close());
  }
  if (closeServerStatusBtn && modalServerStatus) {
    closeServerStatusBtn.addEventListener('click', () => modalServerStatus.close());
  }

  // Random Secret Generator Function
  function generateRandomSecret(length, useUpper, useLower, useDigits, useSymbols) {
    let chars = '';
    if (useUpper) chars += 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
    if (useLower) chars += 'abcdefghijklmnopqrstuvwxyz';
    if (useDigits) chars += '0123456789';
    if (useSymbols) chars += '!@#$%^&*()_+-=[]{}|;:,.<>?';
    if (!chars) chars = 'abcdefghijklmnopqrstuvwxyz0123456789';

    const array = new Uint32Array(length);
    window.crypto.getRandomValues(array);
    let result = '';
    for (let i = 0; i < length; i++) {
      result += chars[array[i] % chars.length];
    }
    return result;
  }

  // HASH GENERATOR & MODAL
  const modalGenerator = document.getElementById('modal-generator');
  const btnOpenGenerator = document.getElementById('btn-open-generator');
  const closeGenerator = document.getElementById('close-generator');
  const cancelGenerator = document.getElementById('cancel-generator');
  const formGeneratorModal = document.getElementById('form-generator-modal');
  const inputPlainSecret = document.getElementById('input-plain-secret');
  const inputHashedSecret = document.getElementById('input-hashed-secret');

  if (btnOpenGenerator && modalGenerator) {
    btnOpenGenerator.addEventListener('click', () => {
      modalGenerator.showModal();
    });
  }
  if (closeGenerator && modalGenerator) {
    closeGenerator.addEventListener('click', () => modalGenerator.close());
  }
  if (cancelGenerator && modalGenerator) {
    cancelGenerator.addEventListener('click', () => modalGenerator.close());
  }

  let hashDebounceTimer = null;
  async function updateHashFromPlain(val) {
    val = (val || '').trim();
    if (!val) {
      if (inputHashedSecret) inputHashedSecret.value = '';
      return;
    }
    try {
      const res = await apiCall('/hash', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ type: 'key', value: val }),
      });
      if (inputHashedSecret) inputHashedSecret.value = res.hash;
    } catch (err) {
      console.warn('Failed to compute hash:', err);
    }
  }

  if (inputPlainSecret) {
    inputPlainSecret.addEventListener('input', () => {
      clearTimeout(hashDebounceTimer);
      hashDebounceTimer = setTimeout(() => {
        updateHashFromPlain(inputPlainSecret.value);
      }, 150);
    });
  }

  if (formGeneratorModal) {
    formGeneratorModal.addEventListener('submit', async (e) => {
      e.preventDefault();
      const lenInput = document.getElementById('gen-length');
      let length = lenInput ? parseInt(lenInput.value, 10) : 32;
      if (isNaN(length) || length < 4) length = 32;
      if (length > 256) length = 256;

      const upper = document.getElementById('gen-opt-upper')?.checked ?? true;
      const lower = document.getElementById('gen-opt-lower')?.checked ?? true;
      const digits = document.getElementById('gen-opt-digits')?.checked ?? true;
      const symbols = document.getElementById('gen-opt-symbols')?.checked ?? true;

      const randomSecret = generateRandomSecret(length, upper, lower, digits, symbols);
      if (inputPlainSecret) {
        inputPlainSecret.value = randomSecret;
      }
      await updateHashFromPlain(randomSecret);
      if (modalGenerator) modalGenerator.close();
    });
  }

  // Database Subtabs (Export SQL / Import SQL)
  document.querySelectorAll('.subtab-btn[data-db-tab]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const tab = btn.getAttribute('data-db-tab');
      document.querySelectorAll('.subtab-btn[data-db-tab]').forEach((b) => {
        b.classList.remove('active');
        b.setAttribute('aria-selected', 'false');
      });
      btn.classList.add('active');
      btn.setAttribute('aria-selected', 'true');

      const expEl = document.getElementById('db-tab-export');
      const impEl = document.getElementById('db-tab-import');
      if (expEl) expEl.classList.toggle('hidden', tab !== 'export');
      if (impEl) impEl.classList.toggle('hidden', tab !== 'import');
    });
  });

  // DB Download SQL
  const btnDBDownload = document.getElementById('btn-db-download');
  if (btnDBDownload) {
    btnDBDownload.addEventListener('click', () => {
      window.location.href = '/api/ui/db/export?download=true';
    });
  }

  // DB Import SQL Form
  const formDBImport = document.getElementById('form-db-import');
  if (formDBImport) {
    formDBImport.addEventListener('submit', (e) => {
      e.preventDefault();
      const fileInput = document.getElementById('input-db-file');
      if (!fileInput || !fileInput.files || fileInput.files.length === 0) {
        showAlert('Please select an SQL file to import.', true);
        return;
      }

      showConfirmDialog({
        title: 'Overwrite Database?',
        prompt: 'Are you sure you want to import this SQL? All existing bots, aliases, delivery history, and tokens in the database will be permanently overwritten and replaced.',
        confirmText: 'Overwrite & Import',
        danger: true,
        onConfirm: async () => {
          try {
            const submitBtn = document.getElementById('btn-db-import-submit');
            if (submitBtn) {
              submitBtn.disabled = true;
              submitBtn.textContent = 'Importing...';
            }

            const formData = new FormData();
            formData.append('file', fileInput.files[0]);
            await apiCall('/db/import', {
              method: 'POST',
              body: formData,
            });

            showAlert('Database imported successfully. All data was replaced.');
            if (formDBImport) formDBImport.reset();

            // Refresh app data
            loadOverview();
            loadBots();
            loadAliases();
            loadHistory();
            loadServerInfo();
          } catch (err) {
            showAlert(`Import failed: ${err.message}`, true);
          } finally {
            const submitBtn = document.getElementById('btn-db-import-submit');
            if (submitBtn) {
              submitBtn.disabled = false;
              submitBtn.textContent = 'Import & Overwrite Database';
            }
          }
        },
      });
    });
  }

  // Copy Buttons
  document.querySelectorAll('.copy-btn').forEach((btn) => {
    btn.addEventListener('click', () => {
      const targetId = btn.getAttribute('data-copy-target');
      const el = document.getElementById(targetId);
      if (!el) return;
      const text = el.textContent || el.value;
      navigator.clipboard.writeText(text).then(() => {
        const orig = btn.textContent;
        btn.textContent = 'Copied!';
        setTimeout(() => { btn.textContent = orig; }, 2000);
      });
    });
  });

  // DELIVERY HISTORY MANAGEMENT
  let historyPageSize = 20;
  let historyHasMore = false;
  let historyLoading = false;

  async function loadHistory(append = false) {
    if (historyLoading) return;
    historyLoading = true;

    const listEl = document.getElementById('history-list');
    const loadMoreBtn = document.getElementById('btn-history-load-more');

    const offset = append ? historyList.length : 0;
    if (loadMoreBtn && append) {
      loadMoreBtn.disabled = true;
      loadMoreBtn.textContent = 'Loading...';
    }

    try {
      const res = await apiCall(`/history?offset=${offset}&limit=${historyPageSize}`);
      const newItems = res.data || [];
      if (res.pageSize) {
        historyPageSize = res.pageSize;
      }
      if (append) {
        historyList = historyList.concat(newItems);
      } else {
        historyList = newItems;
      }
      historyHasMore = !!res.hasMore;

      const totalCount = typeof res.total === 'number' ? res.total : historyList.length;
      const countEl = document.getElementById('tab-history-count');
      if (countEl) countEl.textContent = String(totalCount);

      if (res.retentionDays) {
        historyRetentionDays = res.retentionDays;
        const retEl = document.getElementById('history-retention-text');
        if (retEl) retEl.textContent = String(res.retentionDays);
      }

      renderHistory();
      updateLoadMoreButton();
    } catch (err) {
      if (!append && listEl) {
        listEl.innerHTML = `<div class="empty-state text-danger">Failed to load history: ${escapeHtml(err.message)}</div>`;
      } else {
        showAlert(err.message, true);
      }
    } finally {
      historyLoading = false;
      if (loadMoreBtn) {
        loadMoreBtn.disabled = false;
        loadMoreBtn.textContent = 'Load More Messages';
      }
    }
  }

  function updateLoadMoreButton() {
    const loadMoreContainer = document.getElementById('history-load-more-container');
    if (!loadMoreContainer) return;
    if (historyHasMore) {
      loadMoreContainer.classList.remove('hidden');
    } else {
      loadMoreContainer.classList.add('hidden');
    }
  }

  function renderHistory() {
    const listEl = document.getElementById('history-list');
    if (!listEl) return;

    const typeFilter = (document.getElementById('history-type-filter').value || '').toLowerCase();
    const search = (document.getElementById('history-search-input').value || '').toLowerCase().trim();

    const filtered = historyList.filter((item) => {
      if (typeFilter && item.messageType !== typeFilter) return false;
      if (search) {
        const matchBot = (item.bot || '').toLowerCase().includes(search);
        const matchRcpt = (item.recipient || '').toLowerCase().includes(search);
        const matchChat = (item.chatId || '').toLowerCase().includes(search);
        const matchSummary = (item.summary || '').toLowerCase().includes(search);
        if (!matchBot && !matchRcpt && !matchChat && !matchSummary) return false;
      }
      return true;
    });

    if (filtered.length === 0) {
      listEl.innerHTML = `
        <div class="empty-state">
          ${historyList.length === 0 ? 'No message delivery history yet.' : 'No history matches the selected filters.'}
        </div>`;
      return;
    }

    listEl.innerHTML = filtered.map((item) => {
      let timeStr = item.createdAt || '';
      try {
        const d = new Date(item.createdAt);
        if (!isNaN(d.getTime())) {
          timeStr = d.toLocaleString();
        }
      } catch (_) {}

      return `
        <div class="item-row history-item-row" data-id="${item.id}">
          <div class="item-main flex-1">
            <div class="item-title-row">
              <span class="badge badge-success text-xs">${escapeHtml(item.status || 'delivered')}</span>
              <span class="badge badge-subtle text-xs code-font">${escapeHtml(item.messageType)}</span>
              <span class="item-title code-font">@${escapeHtml(item.bot)}</span>
              <span class="text-muted text-xs">&rarr;</span>
              <span class="code-font text-xs">${escapeHtml(item.recipient)}</span>
              ${item.messageThreadId ? `<span class="badge badge-subtle text-xs">#${item.messageThreadId}</span>` : ''}
            </div>
            <div class="history-summary">
              ${escapeHtml(item.summary || '')}
            </div>
            <div class="item-subtitle text-xs text-muted" style="margin-top: 4px;">
              <span>${escapeHtml(timeStr)}</span>
              ${item.messageId ? ` &bull; Msg ID: <span class="code-font">${item.messageId}</span>` : ''}
              ${item.chatId !== item.recipient ? ` &bull; Chat: <span class="code-font">${escapeHtml(item.chatId)}</span>` : ''}
            </div>
          </div>
          <div class="item-actions">
            <button type="button" class="btn btn-sm btn-subtle btn-action-icon btn-history-detail" data-id="${item.id}" title="View delivery details" aria-label="View delivery details">
              ${ICONS.detail}
            </button>
            <button type="button" class="btn btn-sm btn-danger btn-action-icon btn-history-delete" data-id="${item.id}" title="Delete record" aria-label="Delete record">
              ${ICONS.delete}
            </button>
          </div>
        </div>
      `;
    }).join('');

    // Row event listeners
    listEl.querySelectorAll('.btn-history-detail').forEach((btn) => {
      btn.addEventListener('click', () => {
        const id = parseInt(btn.getAttribute('data-id'), 10);
        const entry = historyList.find((h) => h.id === id);
        if (!entry) return;

        document.getElementById('hist-detail-id').textContent = String(entry.id);
        document.getElementById('hist-detail-time').textContent = entry.createdAt || '-';
        document.getElementById('hist-detail-bot').textContent = '@' + entry.bot;
        document.getElementById('hist-detail-rcpt').textContent = entry.recipient;
        document.getElementById('hist-detail-chat').textContent = entry.chatId;
        document.getElementById('hist-detail-type').textContent = entry.messageType;
        document.getElementById('hist-detail-status').textContent = entry.status;
        document.getElementById('hist-detail-msgid').textContent = entry.messageId ? String(entry.messageId) : '-';

        let p = {};
        let payloadFormatted = '{}';
        if (entry.payload) {
          try {
            p = typeof entry.payload === 'string' ? JSON.parse(entry.payload) : entry.payload;
            payloadFormatted = JSON.stringify(p, null, 2);
          } catch (_) {
            payloadFormatted = String(entry.payload);
          }
        }
        document.getElementById('hist-detail-payload').textContent = payloadFormatted;

        const previewContainer = document.getElementById('hist-detail-preview');
        if (previewContainer) {
          previewContainer.innerHTML = renderTelegramPreview(entry, p);
        }

        const previewTabBtn = document.getElementById('tab-btn-detail-preview');
        if (previewTabBtn) previewTabBtn.click();

        if (typeof modalHistoryDetail.showModal === 'function') {
          modalHistoryDetail.showModal();
        }
      });
    });

    listEl.querySelectorAll('.btn-history-delete').forEach((btn) => {
      btn.addEventListener('click', () => {
        const id = btn.getAttribute('data-id');
        openDeleteHistoryModal(id);
      });
    });
  }

  // Delete History Modal
  let pendingDeleteHistoryId = null;
  const deleteHistoryPrompt = document.getElementById('delete-history-prompt');
  const deleteHistoryTelegram = document.getElementById('delete-history-telegram');
  const deleteHistoryError = document.getElementById('delete-history-error');
  const formDeleteHistory = document.getElementById('form-delete-history');
  const closeDeleteHistBtn = document.getElementById('close-delete-history');
  const cancelDeleteHistBtn = document.getElementById('cancel-delete-history');

  function openDeleteHistoryModal(id) {
    pendingDeleteHistoryId = id;
    if (deleteHistoryPrompt) {
      deleteHistoryPrompt.textContent = `Are you sure you want to delete history record #${id}?`;
    }
    if (deleteHistoryTelegram) {
      deleteHistoryTelegram.checked = false;
    }
    if (deleteHistoryError) {
      deleteHistoryError.textContent = '';
      deleteHistoryError.classList.add('hidden');
    }
    if (modalDeleteHistory && typeof modalDeleteHistory.showModal === 'function') {
      modalDeleteHistory.showModal();
    }
  }

  if (closeDeleteHistBtn) {
    closeDeleteHistBtn.addEventListener('click', () => {
      if (modalDeleteHistory && typeof modalDeleteHistory.close === 'function') modalDeleteHistory.close();
    });
  }
  if (cancelDeleteHistBtn) {
    cancelDeleteHistBtn.addEventListener('click', () => {
      if (modalDeleteHistory && typeof modalDeleteHistory.close === 'function') modalDeleteHistory.close();
    });
  }

  if (formDeleteHistory) {
    formDeleteHistory.addEventListener('submit', (e) => {
      e.preventDefault();
      if (!pendingDeleteHistoryId) return;

      const targetId = Number(pendingDeleteHistoryId);
      const deleteTelegram = deleteHistoryTelegram ? deleteHistoryTelegram.checked : false;
      const endpoint = `/history/${targetId}${deleteTelegram ? '?delete_telegram=true' : ''}`;

      // Find item in historyList to allow rollback if request fails
      const itemIndex = historyList.findIndex((item) => Number(item.id) === targetId);
      const removedItem = itemIndex !== -1 ? historyList[itemIndex] : null;

      // Close modal immediately for instant feedback
      if (modalDeleteHistory && typeof modalDeleteHistory.close === 'function') {
        modalDeleteHistory.close();
      }

      // Optimistic update: remove item from local list and update DOM immediately
      if (removedItem) {
        historyList.splice(itemIndex, 1);
        const countEl = document.getElementById('tab-history-count');
        if (countEl) {
          const current = parseInt(countEl.textContent, 10);
          if (!isNaN(current) && current > 0) {
            countEl.textContent = String(current - 1);
          }
        }
        renderHistory();
        updateLoadMoreButton();
      }

      // Perform API call in background
      apiCall(endpoint, { method: 'DELETE' })
        .then((res) => {
          if (res && res.data && res.data.telegramDeleted) {
            showAlert(`History record #${targetId} and Telegram message deleted.`);
          } else {
            showAlert(`History record #${targetId} removed.`);
          }
        })
        .catch((err) => {
          // Rollback: restore item to historyList and re-render
          if (removedItem) {
            historyList.splice(itemIndex, 0, removedItem);
            const countEl = document.getElementById('tab-history-count');
            if (countEl) {
              const current = parseInt(countEl.textContent, 10);
              if (!isNaN(current)) {
                countEl.textContent = String(current + 1);
              }
            }
            renderHistory();
            updateLoadMoreButton();
          }
          showAlert(err.message, true);
        });
    });
  }

  // History Controls Listeners
  const btnRefreshHistory = document.getElementById('btn-refresh-history');
  if (btnRefreshHistory) {
    btnRefreshHistory.addEventListener('click', () => {
      loadHistory(false);
    });
  }

  const btnHistoryLoadMore = document.getElementById('btn-history-load-more');
  if (btnHistoryLoadMore) {
    btnHistoryLoadMore.addEventListener('click', () => {
      loadHistory(true);
    });
  }

  // Clear History Modal
  const btnClearHistory = document.getElementById('btn-clear-history');
  const formClearHistory = document.getElementById('form-clear-history');
  const closeClearHistBtn = document.getElementById('close-clear-history');
  const cancelClearHistBtn = document.getElementById('cancel-clear-history');

  if (btnClearHistory) {
    btnClearHistory.addEventListener('click', () => {
      if (modalClearHistory && typeof modalClearHistory.showModal === 'function') {
        modalClearHistory.showModal();
      }
    });
  }

  if (closeClearHistBtn) {
    closeClearHistBtn.addEventListener('click', () => {
      if (modalClearHistory && typeof modalClearHistory.close === 'function') modalClearHistory.close();
    });
  }
  if (cancelClearHistBtn) {
    cancelClearHistBtn.addEventListener('click', () => {
      if (modalClearHistory && typeof modalClearHistory.close === 'function') modalClearHistory.close();
    });
  }

  if (formClearHistory) {
    formClearHistory.addEventListener('submit', async (e) => {
      e.preventDefault();
      if (modalClearHistory && typeof modalClearHistory.close === 'function') {
        modalClearHistory.close();
      }
      try {
        await apiCall('/history', { method: 'DELETE' });
        showAlert('All message delivery history cleared.');
        historyList = [];
        historyHasMore = false;
        const countEl = document.getElementById('tab-history-count');
        if (countEl) countEl.textContent = '0';
        renderHistory();
        updateLoadMoreButton();
      } catch (err) {
        showAlert(err.message, true);
      }
    });
  }

  const historyTypeFilter = document.getElementById('history-type-filter');
  if (historyTypeFilter) {
    historyTypeFilter.addEventListener('change', () => {
      renderHistory();
    });
  }

  const historySearchInput = document.getElementById('history-search-input');
  if (historySearchInput) {
    historySearchInput.addEventListener('input', () => {
      renderHistory();
    });
  }

  const closeDetailBtn = document.getElementById('close-history-detail');
  if (closeDetailBtn) {
    closeDetailBtn.addEventListener('click', () => modalHistoryDetail.close());
  }
  const closeDetailBtn2 = document.getElementById('close-history-detail-btn');
  if (closeDetailBtn2) {
    closeDetailBtn2.addEventListener('click', () => modalHistoryDetail.close());
  }

  // History Detail Tabs Switching
  const detailTabBtns = document.querySelectorAll('.detail-tab-btn');
  detailTabBtns.forEach((btn) => {
    btn.addEventListener('click', () => {
      const target = btn.getAttribute('data-tab-target');
      detailTabBtns.forEach((b) => b.classList.remove('active'));
      btn.classList.add('active');
      const panePreview = document.getElementById('detail-pane-preview');
      const panePayload = document.getElementById('detail-pane-payload');
      if (panePreview) panePreview.classList.toggle('hidden', target !== 'preview');
      if (panePayload) panePayload.classList.toggle('hidden', target !== 'payload');
    });
  });

  // Telegram Message Preview Formatter
  function formatTelegramText(text, parseMode) {
    if (!text) return '';
    let escaped = escapeHtml(text);
    if (parseMode === 'HTML') {
      escaped = escaped
        .replace(/&lt;b&gt;(.*?)&lt;\/b&gt;/gi, '<strong>$1</strong>')
        .replace(/&lt;strong&gt;(.*?)&lt;\/strong&gt;/gi, '<strong>$1</strong>')
        .replace(/&lt;i&gt;(.*?)&lt;\/i&gt;/gi, '<em>$1</em>')
        .replace(/&lt;em&gt;(.*?)&lt;\/em&gt;/gi, '<em>$1</em>')
        .replace(/&lt;u&gt;(.*?)&lt;\/u&gt;/gi, '<u>$1</u>')
        .replace(/&lt;s&gt;(.*?)&lt;\/s&gt;/gi, '<s>$1</s>')
        .replace(/&lt;strike&gt;(.*?)&lt;\/strike&gt;/gi, '<s>$1</s>')
        .replace(/&lt;code&gt;(.*?)&lt;\/code&gt;/gi, '<code class="tg-code">$1</code>')
        .replace(/&lt;pre&gt;(.*?)&lt;\/pre&gt;/gi, '<pre class="tg-pre">$1</pre>')
        .replace(/&lt;a href=&quot;(.*?)&quot;&gt;(.*?)&lt;\/a&gt;/gi, '<a href="$1" target="_blank" rel="noopener noreferrer">$2</a>');
    } else if (parseMode === 'MarkdownV2' || parseMode === 'Markdown') {
      escaped = escaped
        .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
        .replace(/\*(.*?)\*/g, '<strong>$1</strong>')
        .replace(/_(.*?)_/g, '<em>$1</em>')
        .replace(/```([\s\S]*?)```/g, '<pre class="tg-pre">$1</pre>')
        .replace(/`([^`]+)`/g, '<code class="tg-code">$1</code>');
    }
    return escaped;
  }

  function renderTelegramPreview(entry, p) {
    let timeShort = '';
    try {
      const d = new Date(entry.createdAt);
      if (!isNaN(d.getTime())) {
        timeShort = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
      }
    } catch (_) {}

    const type = entry.messageType || 'text';
    const metaHtml = `<div class="tg-meta"><span>${escapeHtml(timeShort)}</span></div>`;

    if (type === 'text') {
      const text = (p && p.text) || entry.summary || '';
      const parseMode = p && p.parse_mode;
      return `
        <div class="tg-bubble">
          <div class="tg-text">${formatTelegramText(text, parseMode)}</div>
          ${metaHtml}
        </div>
      `;
    }

    if (type === 'photo') {
      let photoUrl = '';
      let photoLabel = '';
      if (p && p.photo) {
        if (typeof p.photo === 'string') {
          if (p.photo.startsWith('http://') || p.photo.startsWith('https://')) {
            photoUrl = p.photo;
          } else {
            photoLabel = p.photo;
          }
        } else if (p.photo.value && (p.photo.value.startsWith('http://') || p.photo.value.startsWith('https://'))) {
          photoUrl = p.photo.value;
        } else if (p.photo.filename) {
          photoLabel = p.photo.filename;
        } else if (p.photo.value) {
          photoLabel = p.photo.value;
        }
      }
      const caption = p && p.caption;
      const parseMode = p && p.parse_mode;

      let mediaHtml = '';
      if (photoUrl) {
        mediaHtml = `
          <div class="tg-photo-wrap">
            <img src="${escapeHtml(photoUrl)}" alt="Photo preview" class="tg-photo-preview" loading="lazy" onerror="this.parentElement.innerHTML='<div class=\\'tg-media-file\\'><span class=\\'tg-media-icon\\'>🖼</span><span>Photo: ${escapeHtml(photoUrl)}</span></div>';">
          </div>
        `;
      } else {
        mediaHtml = `
          <div class="tg-media-file">
            <span class="tg-media-icon">🖼</span>
            <span>${escapeHtml(photoLabel || entry.summary || 'Photo Attachment')}</span>
          </div>
        `;
      }

      return `
        <div class="tg-bubble">
          ${mediaHtml}
          ${caption ? `<div class="tg-caption">${formatTelegramText(caption, parseMode)}</div>` : ''}
          ${metaHtml}
        </div>
      `;
    }

    if (type === 'document') {
      let docName = '';
      if (p && p.document) {
        if (typeof p.document === 'string') docName = p.document;
        else docName = p.document.filename || p.document.value || '';
      }
      const caption = p && p.caption;
      return `
        <div class="tg-bubble">
          <div class="tg-media-file">
            <span class="tg-media-icon">📄</span>
            <div class="tg-doc-info">
              <strong>${escapeHtml(docName || entry.summary || 'Document')}</strong>
              <span class="text-muted text-xs">Telegram Document</span>
            </div>
          </div>
          ${caption ? `<div class="tg-caption">${formatTelegramText(caption, p && p.parse_mode)}</div>` : ''}
          ${metaHtml}
        </div>
      `;
    }

    if (type === 'video' || type === 'animation') {
      let url = '';
      let label = '';
      const field = type === 'video' ? (p && p.video) : (p && p.animation);
      if (typeof field === 'string') {
        if (field.startsWith('http://') || field.startsWith('https://')) url = field;
        else label = field;
      } else if (field && field.value && (field.value.startsWith('http://') || field.value.startsWith('https://'))) {
        url = field.value;
      } else if (field && (field.filename || field.value)) {
        label = field.filename || field.value;
      }

      const caption = p && p.caption;
      const icon = type === 'animation' ? '🎞' : '🎥';
      let mediaHtml = '';
      if (url) {
        mediaHtml = `
          <div class="tg-photo-wrap">
            <video src="${escapeHtml(url)}" controls class="tg-photo-preview" style="background:#000; max-height:280px;"></video>
          </div>
        `;
      } else {
        mediaHtml = `
          <div class="tg-media-file">
            <span class="tg-media-icon">${icon}</span>
            <span>${escapeHtml(label || (type === 'animation' ? 'GIF / Animation' : 'Video'))}</span>
          </div>
        `;
      }

      return `
        <div class="tg-bubble">
          ${mediaHtml}
          ${caption ? `<div class="tg-caption">${formatTelegramText(caption, p && p.parse_mode)}</div>` : ''}
          ${metaHtml}
        </div>
      `;
    }

    if (type === 'audio' || type === 'voice') {
      let url = '';
      let label = '';
      const field = type === 'audio' ? (p && p.audio) : (p && p.voice);
      if (typeof field === 'string') {
        if (field.startsWith('http://') || field.startsWith('https://')) url = field;
        else label = field;
      } else if (field && field.value && (field.value.startsWith('http://') || field.value.startsWith('https://'))) {
        url = field.value;
      } else if (field && (field.filename || field.value)) {
        label = field.filename || field.value;
      }

      const caption = p && p.caption;
      const icon = type === 'voice' ? '🎙' : '🎵';
      let mediaHtml = '';
      if (url) {
        mediaHtml = `
          <div class="tg-media-file">
            <span class="tg-media-icon">${icon}</span>
            <audio src="${escapeHtml(url)}" controls style="width: 100%; max-width: 280px;"></audio>
          </div>
        `;
      } else {
        mediaHtml = `
          <div class="tg-media-file">
            <span class="tg-media-icon">${icon}</span>
            <span>${escapeHtml(label || (type === 'voice' ? 'Voice Message' : 'Audio File'))}</span>
          </div>
        `;
      }

      return `
        <div class="tg-bubble">
          ${mediaHtml}
          ${caption ? `<div class="tg-caption">${formatTelegramText(caption, p && p.parse_mode)}</div>` : ''}
          ${metaHtml}
        </div>
      `;
    }

    if (type === 'location') {
      const lat = p ? p.latitude : '';
      const lon = p ? p.longitude : '';
      return `
        <div class="tg-bubble">
          <div class="tg-location-card">
            <span class="tg-media-icon">📍</span>
            <div>
              <div style="font-weight:600;">Location Coordinates</div>
              <div class="text-xs code-font text-muted">${escapeHtml(String(lat))}, ${escapeHtml(String(lon))}</div>
              <a href="https://maps.google.com/?q=${encodeURIComponent(lat)},${encodeURIComponent(lon)}" target="_blank" rel="noopener noreferrer" class="tg-map-link">Open in Google Maps &rarr;</a>
            </div>
          </div>
          ${metaHtml}
        </div>
      `;
    }

    if (type === 'poll') {
      const q = (p && p.question) || 'Poll';
      let options = (p && p.options) || [];
      if (!Array.isArray(options)) options = [];
      const optsHtml = options.map((opt) => {
        const text = typeof opt === 'string' ? opt : (opt.text || '');
        return `
          <div class="tg-poll-option">
            <span class="tg-poll-bullet"></span>
            <span>${escapeHtml(text)}</span>
          </div>
        `;
      }).join('');

      return `
        <div class="tg-bubble">
          <div class="tg-poll-card">
            <div class="tg-poll-question">📊 ${escapeHtml(q)}</div>
            <div class="tg-poll-options">${optsHtml}</div>
            <div class="text-xs text-muted" style="margin-top: 6px;">Anonymous Poll</div>
          </div>
          ${metaHtml}
        </div>
      `;
    }

    if (type === 'dice') {
      const emoji = (p && p.emoji) || '🎲';
      return `
        <div class="tg-bubble">
          <div class="tg-dice">${escapeHtml(emoji)}</div>
          ${metaHtml}
        </div>
      `;
    }

    const fallbackText = (p && (p.text || p.caption)) || entry.summary || type;
    return `
      <div class="tg-bubble">
        <div class="tg-text">${escapeHtml(fallbackText)}</div>
        ${metaHtml}
      </div>
    `;
  }

  // Utility
  function escapeHtml(str) {
    if (!str) return '';
    return String(str)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#039;');
  }

  // Initialize
  initTheme();
  checkAuth();
})();
