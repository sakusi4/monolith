document.addEventListener("input", (event) => {
  const input = event.target;
  if (input.form?.id !== "add-item" || input.name !== "name") {
    return;
  }
  const name = input.value.trim().toLowerCase();
  const asset = [...input.list.options].find((option) => option.value.toLowerCase() === name);
  for (const select of input.form.querySelectorAll("select")) {
    select.disabled = asset !== undefined;
    if (asset) {
      select.value = asset.dataset[select.name];
    }
  }
});
