document.addEventListener("input", (event) => {
  const input = event.target;
  if (input.form?.id !== "add-item" || input.name !== "name") {
    return;
  }
  const name = input.value.trim().toLowerCase();
  const suggestion = [...input.list.options].find((option) => option.value.toLowerCase() === name);
  if (!suggestion) {
    return;
  }
  for (const select of input.form.querySelectorAll("select")) {
    select.value = suggestion.dataset[select.name];
  }
});
