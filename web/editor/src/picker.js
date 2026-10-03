import { Plugin, PluginKey } from '@milkdown/kit/prose/state';
import { $prose } from '@milkdown/kit/utils';

const trigger = /\[\[([^[\]\n]*)$/;
const searchDelay = 150;
const createdPage = /^\/page\/pages\/\d+$/;

// pickerPlugin opens a window under "[[text" that finds pages to link and creates subpages of the
// page parentID. say shows a message when creating fails.
export function pickerPlugin({ parentID, say }) {
  return $prose(() => {
    let picker = null;
    return new Plugin({
      key: new PluginKey('page-picker'),
      view: (view) => {
        picker = new Picker(view, parentID, say);
        return { update: (v) => picker.update(v), destroy: () => picker.destroy() };
      },
      props: {
        handleKeyDown: (_view, event) => (picker ? picker.handleKey(event) : false),
      },
    });
  });
}

class Picker {
  constructor(view, parentID, say) {
    this.view = view;
    this.parentID = parentID;
    this.say = say;
    this.items = [];
    this.pages = { query: null, list: [] };
    this.active = 0;
    this.match = null;
    this.dismissed = null;
    this.timer = 0;
    this.searches = 0;
    this.el = document.createElement('div');
    this.el.className = 'page-picker';
    this.el.setAttribute('role', 'listbox');
    this.el.hidden = true;
    this.el.addEventListener('mousedown', (event) => {
      const item = event.target.closest('[data-index]');
      if (item) {
        event.preventDefault();
        this.choose(Number(item.dataset.index));
      }
    });
    document.body.append(this.el);
  }

  update(view) {
    this.view = view;
    const match = find(view.state);
    if (!match) {
      this.dismissed = null;
      this.close();
      return;
    }
    if (match.from === this.dismissed) {
      this.close();
      return;
    }
    const changed = !this.match || this.match.from !== match.from || this.match.query !== match.query;
    this.match = match;
    this.place();
    if (changed) {
      this.search(match.query);
    }
  }

  // search shows the create choice for query at once and the pages found for it once they come.
  search(query) {
    clearTimeout(this.timer);
    const search = ++this.searches;
    this.render(query, this.pages.query === query ? this.pages.list : []);
    this.timer = setTimeout(async () => {
      let pages = [];
      try {
        const res = await fetch('/page/links?q=' + encodeURIComponent(query.trim()));
        if (res.ok) {
          const doc = new DOMParser().parseFromString(await res.text(), 'text/html');
          pages = [...doc.querySelectorAll('#links a')]
            .map((a) => ({
              title: a.dataset.title,
              url: a.getAttribute('href'),
              path: a.nextElementSibling ? a.nextElementSibling.textContent : '',
            }))
            .filter((page) => page.url !== location.pathname);
        }
      } catch {
        pages = [];
      }
      if (search === this.searches && this.match && this.match.query === query) {
        this.pages = { query, list: pages };
        this.render(query, pages);
      }
    }, searchDelay);
  }

  render(query, pages) {
    const title = query.trim();
    this.items = title ? [{ create: true, title }, ...pages] : pages;
    this.active = 0;
    const rows = this.items.map((item, i) => {
      const row = document.createElement('div');
      row.className = 'page-picker-item';
      row.dataset.index = String(i);
      row.setAttribute('role', 'option');
      if (item.create) {
        row.textContent = `Create subpage “${item.title}”`;
      } else {
        const path = document.createElement('small');
        path.textContent = item.path;
        row.append(item.title, ' ', path);
      }
      return row;
    });
    if (rows.length === 0) {
      const empty = document.createElement('div');
      empty.className = 'page-picker-empty';
      empty.textContent = 'Type a page title';
      rows.push(empty);
    }
    this.el.replaceChildren(...rows);
    this.highlight();
    this.el.hidden = false;
    this.place();
  }

  place() {
    if (!this.match) {
      return;
    }
    const at = this.view.coordsAtPos(this.match.from);
    this.el.style.left = `${at.left + window.scrollX}px`;
    this.el.style.top = `${at.bottom + window.scrollY + 4}px`;
  }

  highlight() {
    for (const row of this.el.querySelectorAll('[data-index]')) {
      row.setAttribute('aria-selected', String(Number(row.dataset.index) === this.active));
    }
  }

  handleKey(event) {
    if (this.el.hidden || !this.match) {
      return false;
    }
    switch (event.key) {
      case 'ArrowDown':
        this.active = Math.min(this.active + 1, this.items.length - 1);
        this.highlight();
        return true;
      case 'ArrowUp':
        this.active = Math.max(this.active - 1, 0);
        this.highlight();
        return true;
      case 'Enter':
        if (this.items.length === 0) {
          return false;
        }
        this.choose(this.active);
        return true;
      case 'Escape':
        this.dismissed = this.match.from;
        this.close();
        return true;
      default:
        return false;
    }
  }

  async choose(index) {
    const item = this.items[index];
    const match = this.match;
    if (!item || !match) {
      return;
    }
    this.close();
    this.dismissed = match.from;
    let url = item.url;
    if (item.create) {
      try {
        const res = await fetch('/page/pages/new', {
          method: 'POST',
          body: new URLSearchParams({ parent: this.parentID, title: item.title }),
        });
        url = new URL(res.url).pathname;
        if (!res.redirected || !createdPage.test(url)) {
          throw new Error(`created at ${url}`);
        }
      } catch {
        this.say(`Couldn't create “${item.title}”.`);
        return;
      }
    }
    const { state } = this.view;
    const link = state.schema.text(item.title, [state.schema.marks.link.create({ href: url })]);
    const typed = match.to <= state.doc.content.size && state.doc.textBetween(match.from, match.to) === `[[${match.query}`;
    const tr = typed ? state.tr.replaceWith(match.from, match.to, link) : state.tr.replaceSelectionWith(link, false);
    this.view.dispatch(tr.removeStoredMark(state.schema.marks.link));
    this.view.focus();
  }

  close() {
    clearTimeout(this.timer);
    this.searches++;
    this.match = null;
    this.el.hidden = true;
  }

  destroy() {
    this.close();
    this.el.remove();
  }
}

// find returns the "[[text" right before an empty selection outside code, as the range to replace
// and the text typed after the brackets.
function find(state) {
  const { selection } = state;
  if (!selection.empty) {
    return null;
  }
  const $pos = selection.$from;
  if (!$pos.parent.isTextblock || $pos.parent.type.spec.code) {
    return null;
  }
  const before = $pos.parent.textBetween(0, $pos.parentOffset, undefined, '￼');
  const m = trigger.exec(before);
  if (!m) {
    return null;
  }
  return { from: $pos.pos - m[0].length, to: $pos.pos, query: m[1] };
}
