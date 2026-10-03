import '@milkdown/crepe/theme/common/prosemirror.css';
import '@milkdown/crepe/theme/common/reset.css';
import '@milkdown/crepe/theme/common/block-edit.css';
import '@milkdown/crepe/theme/common/code-mirror.css';
import '@milkdown/crepe/theme/common/cursor.css';
import '@milkdown/crepe/theme/common/link-tooltip.css';
import '@milkdown/crepe/theme/common/list-item.css';
import '@milkdown/crepe/theme/common/placeholder.css';
import '@milkdown/crepe/theme/common/toolbar.css';
import '@milkdown/crepe/theme/common/table.css';
import { CrepeBuilder } from '@milkdown/crepe/builder';
import { blockEdit } from '@milkdown/crepe/feature/block-edit';
import { codeMirror } from '@milkdown/crepe/feature/code-mirror';
import { cursor } from '@milkdown/crepe/feature/cursor';
import { linkTooltip } from '@milkdown/crepe/feature/link-tooltip';
import { listItem } from '@milkdown/crepe/feature/list-item';
import { placeholder } from '@milkdown/crepe/feature/placeholder';
import { table } from '@milkdown/crepe/feature/table';
import { toolbar } from '@milkdown/crepe/feature/toolbar';
import { commandsCtx, editorViewCtx } from '@milkdown/kit/core';
import { upload, uploadConfig } from '@milkdown/kit/plugin/upload';
import { clearTextInCurrentBlockCommand, headingSchema } from '@milkdown/kit/preset/commonmark';
import { remarkGFMPlugin } from '@milkdown/kit/preset/gfm';
import { $remark } from '@milkdown/kit/utils';
import { visit } from 'unist-util-visit';

const pageIcon =
  '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4"/></svg>';

// The image node rejects a null title, which remark gives an image written without one, and the
// parser then drops the image silently.
const imageTitle = $remark('imageTitle', () => () => (tree) => {
  visit(tree, 'image', (node) => {
    node.title ??= '';
  });
});

// Body headings render one level down, "#" as h2, because the page title is the page's h1.
// data-level keeps the level when the editor's own HTML is pasted back in.
function shiftHeadings(ctx) {
  ctx.update(headingSchema.key, (prev) => (c) => {
    const schema = prev(c);
    return {
      ...schema,
      parseDOM: schema.parseDOM.map((rule) => ({
        ...rule,
        getAttrs: (dom) => {
          const attrs = rule.getAttrs(dom);
          return { ...attrs, level: Number(dom.dataset.level) || attrs.level };
        },
      })),
      toDOM: (node) => {
        const [, attrs, hole] = schema.toDOM(node);
        return [`h${Math.min(node.attrs.level + 1, 6)}`, { ...attrs, 'data-level': node.attrs.level }, hole];
      },
    };
  });
}

// The Page item clears the "/…" the user typed and leaves "[[", which opens the page picker.
function addPageItem(builder) {
  builder.addGroup('pages', 'Pages').addItem('page', {
    label: 'Page',
    icon: pageIcon,
    onRun: (ctx) => {
      ctx.get(commandsCtx).call(clearTextInCurrentBlockCommand.key);
      const view = ctx.get(editorViewCtx);
      view.dispatch(view.state.tr.insertText('[['));
    },
  });
}

// createEditor builds the page editor on root with markdown. uploader turns pasted or dropped files
// into nodes, and plugins are added to the editor as they are.
export function createEditor(root, markdown, { uploader, plugins = [] } = {}) {
  const builder = new CrepeBuilder({ root, defaultValue: markdown })
    .addFeature(blockEdit, { buildMenu: addPageItem })
    .addFeature(toolbar)
    .addFeature(linkTooltip)
    .addFeature(listItem)
    .addFeature(table)
    .addFeature(codeMirror)
    .addFeature(cursor)
    .addFeature(placeholder, { text: 'Type / for commands', mode: 'doc' });
  builder.editor
    .config((ctx) => ctx.set(remarkGFMPlugin.options.key, { singleTilde: false }))
    .config(shiftHeadings)
    .use(imageTitle);
  if (uploader) {
    builder.editor
      .config((ctx) => ctx.update(uploadConfig.key, (prev) => ({ ...prev, uploader, enableHtmlFileUploader: false })))
      .use(upload);
  }
  for (const plugin of plugins) {
    builder.editor.use(plugin);
  }
  return builder;
}
