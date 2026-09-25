const canvas = document.getElementById("totals-chart");

if (canvas) {
  const data = JSON.parse(document.getElementById("totals-data").textContent);
  const css = getComputedStyle(document.documentElement);
  const token = (name) => css.getPropertyValue(name).trim();
  const exact = new Intl.NumberFormat("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  const compact = new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 1 });

  const crosshair = {
    id: "crosshair",
    afterDatasetsDraw(chart) {
      const active = chart.getActiveElements();
      if (active.length === 0) {
        return;
      }
      const { ctx, chartArea } = chart;
      ctx.save();
      ctx.strokeStyle = token("--border");
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(active[0].element.x, chartArea.top);
      ctx.lineTo(active[0].element.x, chartArea.bottom);
      ctx.stroke();
      ctx.restore();
    },
  };

  const line = (label, cents, color) => ({
    label,
    data: cents.map((c) => c / 100),
    borderColor: color,
    backgroundColor: color,
    borderWidth: 2,
    pointRadius: 4,
    pointHoverRadius: 6,
    pointHitRadius: 12,
  });

  Chart.defaults.color = token("--muted");
  Chart.defaults.font.family = token("--font");

  new Chart(canvas, {
    type: "line",
    data: {
      labels: data.labels,
      datasets: [
        line("Net worth", data.netWorthCents, token("--series-1")),
        line("Loans", data.loansCents, token("--series-2")),
      ],
    },
    options: {
      maintainAspectRatio: false,
      interaction: { mode: "index", intersect: false },
      plugins: {
        legend: {
          align: "start",
          labels: { usePointStyle: true, pointStyle: "circle", boxWidth: 8, boxHeight: 8 },
        },
        tooltip: {
          usePointStyle: true,
          callbacks: { label: (item) => item.dataset.label + ": USD " + exact.format(item.parsed.y) },
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
    plugins: [crosshair],
  });
}
