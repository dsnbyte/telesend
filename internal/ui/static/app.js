// Telesend Web Console JS
(function () {
  'use strict';

  // State
  let botsList = [];
  let aliasesList = [];
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
  const modalEditBot = document.getElementById('modal-edit-bot');
  const modalEditAlias = document.getElementById('modal-edit-alias');

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
    [modalLogin, modalEditBot, modalEditAlias].forEach((m) => {
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
    await Promise.allSettled([loadBots(), loadAliases(), loadServerInfo()]);
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
          <button type="button" class="btn btn-sm btn-subtle btn-edit-bot" data-username="${escapeHtml(bot.username)}" data-name="${escapeHtml(bot.name)}" title="Edit bot">
            Edit
          </button>
          <button type="button" class="btn btn-sm btn-danger btn-remove-bot" data-username="${escapeHtml(bot.username)}" title="Remove bot">
            Remove
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
      btn.addEventListener('click', async () => {
        const username = btn.getAttribute('data-username');
        if (!confirm(`Remove bot @${username}?`)) return;
        try {
          await apiCall(`/bots/${encodeURIComponent(username)}`, { method: 'DELETE' });
          showAlert(`Bot @${username} removed.`);
          loadBots();
        } catch (err) {
          showAlert(err.message, true);
        }
      });
    });
  }

  // Add Bot Form
  const addBotCard = document.getElementById('add-bot-card');
  document.getElementById('toggle-add-bot-btn').addEventListener('click', () => {
    addBotCard.classList.toggle('hidden');
    if (!addBotCard.classList.contains('hidden')) {
      document.getElementById('bot-token-input').focus();
    }
  });
  document.getElementById('cancel-add-bot').addEventListener('click', () => {
    addBotCard.classList.add('hidden');
  });

  document.getElementById('form-add-bot').addEventListener('submit', async (e) => {
    e.preventDefault();
    const token = document.getElementById('bot-token-input').value.trim();
    if (!token) return;

    try {
      const res = await apiCall('/bots', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token }),
      });
      document.getElementById('form-add-bot').reset();
      addBotCard.classList.add('hidden');
      showAlert(`Bot @${res.data.username} registered.`);
      loadBots();
    } catch (err) {
      showAlert(err.message, true);
    }
  });

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
          <button type="button" class="btn btn-sm btn-subtle btn-edit-alias"
            data-name="${escapeHtml(al.name)}"
            data-chat-id="${escapeHtml(al.chatId)}"
            data-thread="${al.messageThreadId || ''}"
            title="Edit alias">
            Edit
          </button>
          <button type="button" class="btn btn-sm btn-danger btn-remove-alias" data-name="${escapeHtml(al.name)}" title="Delete alias">
            Delete
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
      btn.addEventListener('click', async () => {
        const name = btn.getAttribute('data-name');
        if (!confirm(`Delete alias "${name}"?`)) return;
        try {
          await apiCall(`/aliases/${encodeURIComponent(name)}`, { method: 'DELETE' });
          showAlert(`Alias "${name}" deleted.`);
          loadAliases();
        } catch (err) {
          showAlert(err.message, true);
        }
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

  // Add Alias Form
  const addAliasCard = document.getElementById('add-alias-card');
  document.getElementById('toggle-add-alias-btn').addEventListener('click', () => {
    addAliasCard.classList.toggle('hidden');
    if (!addAliasCard.classList.contains('hidden')) {
      document.getElementById('alias-name-input').focus();
    }
  });
  document.getElementById('cancel-add-alias').addEventListener('click', () => {
    addAliasCard.classList.add('hidden');
  });

  document.getElementById('form-add-alias').addEventListener('submit', async (e) => {
    e.preventDefault();
    const name = document.getElementById('alias-name-input').value.trim();
    const chatId = document.getElementById('alias-chat-id-input').value.trim();
    const threadStr = document.getElementById('alias-thread-id-input').value.trim();
    const messageThreadId = threadStr ? parseInt(threadStr, 10) : null;

    try {
      await apiCall('/aliases', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, chatId, messageThreadId }),
      });
      document.getElementById('form-add-alias').reset();
      addAliasCard.classList.add('hidden');
      showAlert(`Alias "${name}" created.`);
      loadAliases();
    } catch (err) {
      showAlert(err.message, true);
    }
  });

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

  // Media Source Toggle (Upload vs URL)
  let activeMediaSource = 'upload';
  document.querySelectorAll('.segment-btn[data-media-src]').forEach((btn) => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('.segment-btn[data-media-src]').forEach((b) => b.classList.remove('active'));
      btn.classList.add('active');
      activeMediaSource = btn.getAttribute('data-media-src');
      const isUpload = activeMediaSource === 'upload';
      document.getElementById('media-upload-group').classList.toggle('hidden', !isUpload);
      document.getElementById('media-url-group').classList.toggle('hidden', isUpload);
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
        if (document.getElementById('send-media-silent').checked) fd.append('silent', 'true');
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
        let silent = false;

        if (typ === 'text') {
          const text = document.getElementById('send-text-content').value;
          if (!text) throw new Error('Message text is required.');
          payload.text = text;
          const parseMode = document.getElementById('send-parse-mode').value;
          if (parseMode) payload.parse_mode = parseMode;
          silent = document.getElementById('send-silent').checked;
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
          silent = document.getElementById('send-media-silent').checked;
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
      document.getElementById('srv-version').textContent = `v${res.version}`;
      document.getElementById('srv-bot-api').textContent = `Telegram Bot API ${res.botApiVersion}`;
      document.getElementById('srv-db-file').textContent = res.databaseFile;

      const toolsListEl = document.getElementById('mcp-tools-list');
      if (res.mcpTools && res.mcpTools.length > 0) {
        toolsListEl.innerHTML = res.mcpTools.map((t) => `
          <div class="mcp-tool-card">
            <div class="mcp-tool-name">${escapeHtml(t.name)}</div>
            <div class="mcp-tool-desc">${escapeHtml(t.description)}</div>
          </div>
        `).join('');
      }
    } catch (err) {
      console.warn('Failed to load server info', err);
    }
  }

  // TOOLS: HASH GENERATORS
  document.getElementById('form-hash-key').addEventListener('submit', async (e) => {
    e.preventDefault();
    const val = document.getElementById('input-api-key').value;
    if (!val) return;
    try {
      const res = await apiCall('/hash', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ type: 'key', value: val }),
      });
      document.getElementById('hash-key-text').textContent = res.envLine;
      document.getElementById('hash-key-output').classList.remove('hidden');
    } catch (err) {
      showAlert(err.message, true);
    }
  });

  document.getElementById('form-hash-password').addEventListener('submit', async (e) => {
    e.preventDefault();
    const val = document.getElementById('input-mcp-password').value;
    if (!val) return;
    try {
      const res = await apiCall('/hash', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ type: 'password', value: val }),
      });
      document.getElementById('hash-password-text').textContent = res.envLine;
      document.getElementById('hash-password-output').classList.remove('hidden');
    } catch (err) {
      showAlert(err.message, true);
    }
  });

  const formHashUIPw = document.getElementById('form-hash-ui-password');
  if (formHashUIPw) {
    formHashUIPw.addEventListener('submit', async (e) => {
      e.preventDefault();
      const val = document.getElementById('input-ui-password').value;
      if (!val) return;
      try {
        const res = await apiCall('/hash', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ type: 'ui-password', value: val }),
        });
        document.getElementById('hash-ui-password-text').textContent = res.envLine;
        document.getElementById('hash-ui-password-output').classList.remove('hidden');
      } catch (err) {
        showAlert(err.message, true);
      }
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
