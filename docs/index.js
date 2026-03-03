    // Random gopher color
    const svg = document.querySelector('.gopher-svg');
    const hue = Math.floor(Math.random() * 360);
    const gopherColor = 'hsl(' + hue + ', 45%, 75%)';
    svg.querySelectorAll('[fill="#6AD7E5"]').forEach(el => el.setAttribute('fill', gopherColor));

    // Parallax
    if (svg) window.addEventListener('scroll', () => {
      svg.style.transform = 'translateY(' + window.scrollY * 0.35 + 'px)';
    }, { passive: true });

    // Eye tracking
    const eyes = [
      { g: document.querySelector('[data-eye="left"]'),  cx: 130, cy: 85, rx: 12, ry: 20 },
      { g: document.querySelector('[data-eye="right"]'), cx: 254, cy: 81, rx: 12, ry: 20 },
    ];
    document.addEventListener('mousemove', (e) => {
      const rect = svg.getBoundingClientRect();
      const scaleX = 401.98 / rect.width;
      const scaleY = 559.472 / rect.height;
      const mx = (e.clientX - rect.left) * scaleX;
      const my = (e.clientY - rect.top) * scaleY;
      for (const eye of eyes) {
        const dx = mx - eye.cx, dy = my - eye.cy;
        const angle = Math.atan2(dy, dx);
        const dist = Math.min(1, Math.hypot(dx, dy) / 80);
        const ox = Math.cos(angle) * eye.rx * dist;
        const oy = Math.sin(angle) * eye.ry * dist;
        const els = eye.g.children;
        els[0].setAttribute('cx', eye.cx + ox);
        els[0].setAttribute('cy', eye.cy + oy);
        els[1].setAttribute('cx', eye.cx + ox + 6);
        els[1].setAttribute('cy', eye.cy + oy + 4);
      }
    }, { passive: true });

    // Blink
    const tops = svg.querySelectorAll('.lid-top');
    const bots = svg.querySelectorAll('.lid-bot');
    function blink(twice) {
      tops.forEach(l => l.style.transform = 'translateY(250px)');
      bots.forEach(l => l.style.transform = 'translateY(-250px)');
      setTimeout(() => {
        tops.forEach(l => l.style.transform = '');
        bots.forEach(l => l.style.transform = '');
        if (twice) setTimeout(() => blink(false), 200 + Math.random() * 150);
      }, 150);
    }
    setTimeout(() => blink(false), 1000);
    svg.addEventListener('click', () => blink(Math.random() < 0.5));
    (function loop() {
      setTimeout(() => { blink(Math.random() < 0.5); loop(); }, 2500 + Math.random() * 4000);
    })();
