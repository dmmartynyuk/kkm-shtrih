function data() {
	/*
  function getThemeFromLocalStorage() {
    // if user already changed the theme, use it
    if (window.localStorage.getItem('dark')) {
      return JSON.parse(window.localStorage.getItem('dark'))
    }
    // else return their preferences
    return (
      !!window.matchMedia &&
      window.matchMedia('(prefers-color-scheme: dark)').matches
    )
  }
  function setThemeToLocalStorage(value) {
    window.localStorage.setItem('dark', value)
  }*/

  return {
	  /*
    dark: getThemeFromLocalStorage(),
    toggleTheme() {
      this.dark = !this.dark
      setThemeToLocalStorage(this.dark)
    },*/
	getServSettings: function() {
		fetch("/api/GetServSetting",
			{
			  method: "GET", // POST, PUT, DELETE, etc.
			  headers: {
				// значение этого заголовка обычно ставится автоматически,
				// в зависимости от тела запроса
				"Content-Type": "application/json;charset=UTF-8"
			  },
			  //body: undefined // string, FormData, Blob, BufferSource или URLSearchParams
			  //referrer: "about:client", // или "" для того, чтобы не послать заголовок Referer,
			  // или URL с текущего источника
			  //referrerPolicy: "no-referrer-when-downgrade", // unsafe-url no-referrer, origin, same-origin...
			  //mode: 'cors', // same-origin, no-cors
			  //credentials: "same-origin", // omit, include
			  cache: "no-store", // no-store, reload, no-cache, force-cache или only-if-cached
			  //redirect: "follow", // manual, error
			  //integrity: "", // контрольная сумма, например "sha256-abcdef1234567890"
			  //keepalive: false, // true
			  //signal: undefined, // AbortController, чтобы прервать запрос
			  //window: window // null
			}
		)
		.then(response => response.text())
		.then(data => this.kkndata=data)
		.catch(err => console.log(err));
	},
	kkmdata: {},
    isSideMenuOpen: false,
    toggleSideMenu() {
      this.isSideMenuOpen = !this.isSideMenuOpen
    },
    closeSideMenu() {
      this.isSideMenuOpen = false
    },
    isNotificationsMenuOpen: false,
    toggleNotificationsMenu() {
      this.isNotificationsMenuOpen = !this.isNotificationsMenuOpen
    },
    closeNotificationsMenu() {
      this.isNotificationsMenuOpen = false
    },
    isProfileMenuOpen: false,
    toggleProfileMenu() {
      this.isProfileMenuOpen = !this.isProfileMenuOpen
    },
    closeProfileMenu() {
      this.isProfileMenuOpen = false
    },
    isPagesMenuOpen: false,
    togglePagesMenu() {
      this.isPagesMenuOpen = !this.isPagesMenuOpen
    },
    // Modal
    isModalOpen: false,
    trapCleanup: null,
    openModal() {
      this.isModalOpen = true
      this.trapCleanup = focusTrap(document.querySelector('#modal'))
    },
    closeModal() {
      this.isModalOpen = false
      this.trapCleanup()
    },
  }
}
