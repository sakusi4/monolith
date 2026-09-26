const css = getComputedStyle(document.documentElement);
const token = (name) => css.getPropertyValue(name).trim();
const exact = new Intl.NumberFormat("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
const compact = new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 1 });

Chart.defaults.color = token("--muted");
Chart.defaults.font.family = token("--font");

function drawSpendingChart() {
  const canvas = document.getElementById("spending-chart");
  if (!canvas || Chart.getChart(canvas)) {
    return;
  }
  const data = JSON.parse(document.getElementById("spending-data").textContent);
  new Chart(canvas, {
    type: "bar",
    data: {
      labels: data.labels,
      datasets: [
        {
          label: "Total",
          data: data.totalCents.map((c) => c / 100),
          backgroundColor: token("--series-1"),
          borderRadius: 4,
          maxBarThickness: 32,
        },
      ],
    },
    options: {
      maintainAspectRatio: false,
      interaction: { mode: "index", intersect: false },
      plugins: {
        legend: { display: false },
        tooltip: {
          displayColors: false,
          callbacks: { label: (item) => "Total: USD " + exact.format(item.parsed.y) },
        },
      },
      scales: {
        x: { grid: { display: false }, ticks: { maxRotation: 0, autoSkipPadding: 12 } },
        y: {
          border: { display: false },
          grid: { color: token("--border") },
          ticks: { maxTicksLimit: 5, callback: (value) => "USD " + compact.format(value) },
        },
      },
    },
  });
}

drawSpendingChart();
document.addEventListener("htmx:load", drawSpendingChart);
