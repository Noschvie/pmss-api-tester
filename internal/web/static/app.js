(() => {
	const root = document.documentElement;
	const savedTheme = localStorage.getItem("pmss-theme");
	if (savedTheme === "light" || savedTheme === "dark") {
		root.dataset.theme = savedTheme;
	}

	document.addEventListener("click", (event) => {
		const target = event.target;
		if (!(target instanceof Element)) return;
		const toggle = target.closest(".theme-toggle");
		if (!toggle) return;
		const theme = root.dataset.theme === "dark" ? "light" : "dark";
		root.dataset.theme = theme;
		localStorage.setItem("pmss-theme", theme);
	});
})();
