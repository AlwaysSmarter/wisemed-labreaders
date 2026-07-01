async function loadHealth() {
  const target = document.getElementById("health");
  try {
    const response = await fetch("/api/signing-pad/health", { credentials: "include" });
    const data = await response.json();
    target.textContent = JSON.stringify(data, null, 2);
  } catch (error) {
    target.textContent = String(error);
  }
}

loadHealth();
