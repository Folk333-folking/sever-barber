// Mobile nav close on link click
document.querySelectorAll('nav a').forEach(a => {
  a.addEventListener('click', () => {
    document.body.classList.remove('nav-open');
  });
});
