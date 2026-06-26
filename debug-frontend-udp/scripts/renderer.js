(function () {
  const { CONFIG } = window.EnservaDebug;

  function createRenderer(canvas) {
    const ctx = canvas.getContext("2d");

    function drawGrid() {
      ctx.strokeStyle = CONFIG.colors.grid;

      for (let x = 0; x < canvas.width; x += CONFIG.canvas.gridSize) {
        ctx.beginPath();
        ctx.moveTo(x, 0);
        ctx.lineTo(x, canvas.height);
        ctx.stroke();
      }

      for (let y = 0; y < canvas.height; y += CONFIG.canvas.gridSize) {
        ctx.beginPath();
        ctx.moveTo(0, y);
        ctx.lineTo(canvas.width, y);
        ctx.stroke();
      }
    }

    function drawPlayer(player, color) {
      ctx.beginPath();
      ctx.arc(player.x, player.y, CONFIG.canvas.playerRadius, 0, Math.PI * 2);
      ctx.fillStyle = color;
      ctx.fill();
    }

    function drawStats(localPlayer, remotePlayers) {
      const totalPlayers = (localPlayer.id ? 1 : 0) + remotePlayers.size;

      ctx.fillStyle = "#e2e8f0";
      ctx.font = "14px system-ui, sans-serif";
      ctx.fillText(`Players: ${totalPlayers}  Remotes: ${remotePlayers.size}`, 12, 22);
    }

    return {
      draw({ localPlayer, remotePlayers }) {
        ctx.clearRect(0, 0, canvas.width, canvas.height);
        drawGrid();

        remotePlayers.forEach((player) => drawPlayer(player, CONFIG.colors.remotePlayer));
        drawPlayer(localPlayer, CONFIG.colors.localPlayer);
        drawStats(localPlayer, remotePlayers);
      }
    };
  }

  window.EnservaDebug = {
    ...window.EnservaDebug,
    createRenderer
  };
}());
