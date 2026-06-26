(function () {
  function isInBoxHitbox(x1, y1, x2, y2) {
    return x1 < x2 && y1 < y2;
  }

  function isAtEdgeOfBoxHitbox(x1, y1, x2, y2) {
    return x1 === x2 || y1 === y2;
  }

  function arePlayersColliding(player1, player2) {
    const radius = window.EnservaDebug.CONFIG.canvas.playerRadius;
    const dx = player1.x - player2.x;
    const dy = player1.y - player2.y;
    const distance = Math.sqrt(dx * dx + dy * dy);
    return distance < radius * 2;
  }

  window.EnservaDebug = {
    ...window.EnservaDebug,
    isInBoxHitbox,
    isAtEdgeOfBoxHitbox,
    arePlayersColliding,
  };
})();
