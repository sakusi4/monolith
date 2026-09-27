const pastedName = (file, index) => {
  const ext = (file.type.split("/")[1] || "png").replace("jpeg", "jpg");
  return `pasted-${Date.now()}-${index + 1}.${ext}`;
};

const linkName = (name) => name.replace(/[\s()<>[\]\\]/g, "-");

const linkTo = (name) => `](${name})`;

function addFiles(area, files, rename) {
  const input = area.form.elements.files;
  const pending = new DataTransfer();
  const taken = new Set();
  for (const file of input.files) {
    pending.items.add(file);
    taken.add(file.name);
  }
  let text = "";
  files.forEach((file, i) => {
    let name = rename ? pastedName(file, i) : linkName(file.name);
    if (taken.has(name)) {
      name = `${Date.now()}-${name}`;
    }
    taken.add(name);
    pending.items.add(new File([file], name, { type: file.type }));
    text += `${file.type.startsWith("image/") ? "!" : ""}[${name}${linkTo(name)}\n`;
  });
  input.files = pending.files;
  const { selectionStart: start, selectionEnd: end, value } = area;
  area.value = value.slice(0, start) + text + value.slice(end);
  area.selectionStart = area.selectionEnd = start + text.length;
  area.focus();
}

const attachArea = (target) => (target instanceof Element ? target.closest("textarea[data-attach]") : null);

document.addEventListener("paste", (event) => {
  const area = attachArea(event.target);
  const files = [...(event.clipboardData?.files ?? [])];
  if (!area || files.length === 0 || event.clipboardData.types.includes("text/plain")) {
    return;
  }
  event.preventDefault();
  addFiles(area, files, true);
});

document.addEventListener("dragover", (event) => {
  if (attachArea(event.target) && event.dataTransfer?.types.includes("Files")) {
    event.preventDefault();
  }
});

document.addEventListener("drop", (event) => {
  const area = attachArea(event.target);
  const files = [...(event.dataTransfer?.files ?? [])];
  if (!area || files.length === 0) {
    return;
  }
  event.preventDefault();
  addFiles(area, files, false);
});

document.addEventListener(
  "submit",
  (event) => {
    const area = event.target.querySelector?.("textarea[data-attach]");
    if (!area) {
      return;
    }
    const input = area.form.elements.files;
    const kept = new DataTransfer();
    for (const file of input.files) {
      if (area.value.includes(linkTo(file.name))) {
        kept.items.add(file);
      }
    }
    input.files = kept.files;
  },
  true,
);
