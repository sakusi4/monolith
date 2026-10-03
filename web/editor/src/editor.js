import { createEditor } from './config.js';
import { pickerPlugin } from './picker.js';

const saveDelay = 1000;
const maxWait = 5000;
const retryDelay = 5000;
const keepaliveLimit = 60000;
const internalLink = /^\/(page\/pages|task\/(tasks|projects))\/\d+/;
const loginPath = '/auth/login';

const root = document.querySelector('[data-editor]');
if (root) {
  start(root);
}

async function start(root) {
  const titleInput = document.querySelector('[data-editor-title]');
  const status = document.querySelector('[data-editor-status]');
  const mount = root.querySelector('[data-editor-mount]');
  const say = (text) => {
    status.textContent = text;
  };
  const state = {
    version: root.dataset.version,
    saved: { title: titleInput.value.trim(), body: '' },
    rejected: { title: null, problem: '' },
    dirtySince: 0,
    saving: null,
    sending: null,
    again: false,
    stopped: false,
    timer: 0,
  };

  const uploader = async (files, schema) => {
    const nodes = [];
    for (const file of files) {
      const name = uploadName(file);
      const form = new FormData();
      form.append('files', file, name);
      try {
        const res = await fetch(root.dataset.imagesUrl, { method: 'POST', body: form });
        if (isSignedOut(res)) {
          say(`Signed out. Sign in in another tab, then add ${name} again.`);
          continue;
        }
        if (res.status !== 201) {
          const problem = res.status >= 400 && res.status < 500 ? (await res.text()).trim() : '';
          say(problem || `Couldn't upload ${name}.`);
          continue;
        }
        const url = res.headers.get('Location');
        nodes.push(
          file.type.startsWith('image/')
            ? schema.nodes.image.create({ src: `${url}/content`, alt: name, title: '' })
            : schema.text(name, [schema.marks.link.create({ href: url })]),
        );
      } catch {
        say(`Couldn't upload ${name}.`);
      }
    }
    return nodes;
  };

  const editor = createEditor(mount, root.querySelector('textarea').value, {
    uploader,
    plugins: [pickerPlugin({ parentID: root.dataset.pageId, say })],
  });
  editor.on((listener) => listener.markdownUpdated(() => schedule()));
  await editor.create();
  state.saved.body = editor.getMarkdown();

  const current = () => ({ title: titleInput.value.trim(), body: editor.getMarkdown() });
  const same = (a, b) => a.title === b.title && a.body === b.body;
  const isDirty = () => !same(current(), state.saved);
  const stop = (text) => {
    state.stopped = true;
    say(text);
  };

  // schedule saves a second after the last change, and at least every maxWait while changes keep coming.
  function schedule() {
    clearTimeout(state.timer);
    if (state.stopped || !isDirty()) {
      return;
    }
    state.dirtySince ||= Date.now();
    state.timer = setTimeout(save, Math.min(saveDelay, Math.max(0, state.dirtySince + maxWait - Date.now())));
  }

  // fitsKeepalive reports whether a save of c fits in the request size a browser sends while leaving.
  const fitsKeepalive = (c) => new Blob([c.title, c.body]).size < keepaliveLimit;

  async function save() {
    clearTimeout(state.timer);
    if (state.stopped || !isDirty()) {
      return;
    }
    if (state.saving) {
      state.again = true;
      return state.saving;
    }
    state.sending = current();
    const c = { ...state.sending };
    let note = '';
    if (!c.title) {
      c.title = state.saved.title;
      note = 'Enter a title.';
    } else if (c.title === state.rejected.title) {
      c.title = state.saved.title;
      note = state.rejected.problem;
    }
    say('Saving…');
    state.saving = send(c, note);
    await state.saving;
    state.saving = null;
    if (state.again) {
      state.again = false;
      schedule();
    }
  }

  async function send(c, note) {
    const form = new FormData();
    form.append('title', c.title);
    form.append('body', c.body);
    form.append('version', state.version);
    try {
      const res = await fetch(root.dataset.contentUrl, { method: 'POST', body: form, keepalive: fitsKeepalive(c) });
      if (isSignedOut(res)) {
        say('Signed out. Sign in in another tab to keep saving.');
        state.timer = setTimeout(save, retryDelay);
        return;
      }
      if (res.status === 204) {
        state.version = res.headers.get('Page-Version');
        state.saved = c;
        state.dirtySince = 0;
        say(note ? `Saved. ${note}` : 'Saved');
        return;
      }
      if (res.status === 409) {
        stop('This page changed elsewhere. Reload to keep editing.');
        return;
      }
      if (res.status === 422) {
        state.rejected = { title: c.title, problem: (await res.text()).trim() };
        say(state.rejected.problem);
        state.timer = setTimeout(save, 0);
        return;
      }
      if (res.status === 404) {
        stop('This page was deleted elsewhere. Copy any text you need before leaving.');
        return;
      }
      if (res.status < 500) {
        stop("Couldn't save. Reload to keep editing.");
        return;
      }
      throw new Error(`status ${res.status}`);
    } catch {
      say("Couldn't save. Retrying…");
      state.timer = setTimeout(save, retryDelay);
    }
  }

  titleInput.addEventListener('input', schedule);
  titleInput.addEventListener('keydown', (event) => {
    if (event.key === 'Enter' && !event.isComposing && event.keyCode !== 229) {
      event.preventDefault();
      mount.querySelector('[contenteditable]').focus();
    }
  });
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') {
      save();
    }
  });
  window.addEventListener('pagehide', () => save());
  window.addEventListener('beforeunload', (event) => {
    if (!isDirty()) {
      return;
    }
    const queued = state.saving && !same(current(), state.sending);
    save();
    if (queued || !fitsKeepalive(current())) {
      event.preventDefault();
    }
  });
  mount.addEventListener(
    'click',
    async (event) => {
      const a = event.target.closest('a[href]');
      if (!a) {
        return;
      }
      const href = a.getAttribute('href');
      const modified = event.metaKey || event.ctrlKey;
      if (internalLink.test(href) && !modified && !event.shiftKey) {
        event.preventDefault();
        event.stopPropagation();
        await save();
        location.href = href;
      } else if (modified) {
        event.preventDefault();
        event.stopPropagation();
        window.open(a.href, '_blank', 'noopener');
      }
    },
    true,
  );
}

// isSignedOut reports whether res is the sign-in page, where a request lands once the session has ended.
function isSignedOut(res) {
  return res.redirected && new URL(res.url).pathname === loginPath;
}

// uploadName names a pasted image, which the browser calls image.png, after the time it came.
function uploadName(file) {
  if (file.name && !/^image\.\w+$/.test(file.name)) {
    return file.name;
  }
  const ext = (file.type.split('/')[1] || 'png').replace('jpeg', 'jpg');
  return `pasted-${Date.now()}.${ext}`;
}
