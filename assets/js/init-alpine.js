function data() {
	function getcurKKMFromLocalStorage() {
    if (window.localStorage.getItem('currentkkm')) {
      return (window.localStorage.getItem('currentkkm'))
    }
     return ""
  }
  function setcurKKMToLocalStorage(value) {
    window.localStorage.setItem('currentkkm', value)
  }

  return {
	  showSuccessMessage:false,
	  showAlertMessage:false,
	  showSearchKKM:false,
	  tooltip:false,
	  currentkkm:'',
	  command:'',
	  retdata:'',
	  kkmerr:'',
	  isError:false,
	  errormsg:"",
	  successmsg:"",
	  servports: [], //[com1,com2...]
	  bauds:[2400, 4800, 9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600],	
	  fnddkkm: [], //"baud":,"port":,"device","err"
	getServPorts: function() { //поиск портов сервера
		this.showSearchKKM=true;
		fetch("/api/getPorts",
			{
			  method: "GET", // POST, PUT, DELETE, etc.
			  headers: {
				"Content-Type": "application/json;charset=UTF-8"
			  },
			  cache: "no-store", // no-store, reload, no-cache, force-cache или only-if-cached
			}
		)
		.then(response => response.json())
		.then(data => {this.servports=data.ports;
			if(data.ports.length==0){
				if(data.os=='windows'){
					for(let i=1; i<16;i++){this.servports.push("com"+i);}
				}else{
					for(let i=0; i<16;i++){this.servports.push("/dev/ttyS"+i);}
					for(let i=0; i<16;i++){this.servports.push("/dev/ttyUSB"+i);}
				}
			}else{
				let pname = new Set();
				for(let i=0; i<data.ports.length;i++){
					pname.add(data.ports[i].port);
				}
				this.servports=[...pname].sort();
			}
			this.showSearchKKM=false;
		})
		.catch(err => console.log(err));
	},
	searchKKM: function() { //поиск ккм сервера
		this.showSearchKKM=true;
		fetch("/api/SearchKKM",
			{
			  method: "GET",
			  headers: {
				"Content-Type": "application/json;charset=UTF-8"
			  },
			  cache: "no-store", // no-store, reload, no-cache, force-cache или only-if-cached
			}
		)
		.then(response => response.json())
		.then(data => {this.fnddkkm=data.devices;
			if(this.fnddkkm.length>0){
				this.showSuccessMessage=true;
				let msg="";
				for(let i=0;i<data.devices.length;i++){msg=msg+data.devices[i].device+". порт: "+data.devices[i].port+", baud: "+data.devices[i].baud+"<br />";}
				this.successmsg=msg; //"baud":,"port":,"device","err"
			}else{
				this.errormsg="ККМ не найдены :-(";this.isError=true;this.showAlertMessage=true;
			}
			this.showSearchKKM=false;
		})
		.catch(err => {console.log(err);this.showSearchKKM=false;});
	},
	getServSettings: function() {
		this.getServPorts();
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
		.then(response => response.json())
		.then(data => {this.kkmsdata=data;this.kkmids=data.deviceids;if(getcurKKMFromLocalStorage()!='')this.currentkkm=getcurKKMFromLocalStorage();else if(data.deviceids.length>0)this.currentkkm=data.deviceids[0];})
		.catch(err => console.log(err));
	},
	setServSettings: function() {
		setcurKKMToLocalStorage(this.currentkkm);
		this.kkmdata.maxattempt=Number(this.kkmdata.maxattempt);
		this.kkmdata.adminpassword=Number(this.kkmdata.adminpassword);
		this.kkmdata.password=Number(this.kkmdata.password);
		this.kkmdata.timeout=Number(this.kkmdata.timeout);
		this.kkmdata.portconf.baud=Number(this.kkmdata.portconf.baud);
		this.kkmdata.portconf.readtimeout=Number(this.kkmdata.portconf.readtimeout);
		this.kkmdata.portconf.size=Number(this.kkmdata.portconf.size);
		this.kkmdata.portconf.parity=Number(this.kkmdata.portconf.parity);
		this.kkmdata.portconf.stopbits=Number(this.kkmdata.portconf.stopbits);
		this.kkmdata.portconf.startbits=Number(this.kkmdata.portconf.startbits);
		fetch("/api/SetServSetting",
			{
			  method: "PUT", // POST, PUT, DELETE, etc.
			  headers: {
				"Content-Type": "application/json;charset=UTF-8"
			  },
			  body: JSON.stringify(this.kkmdata), // undefined, string, FormData, Blob, BufferSource или URLSearchParams
			  cache: "no-store", // no-store, reload, no-cache, force-cache или only-if-cached
			}
		)
		.then(response => response.json())
		.then(data => {if(data.error){this.errormsg=data.message;this.isError=true;this.showAlertMessage=true;return;};this.successmsg="Все изменения записаны!";this.showSuccessMessage=true;this.kkmsdata=data;this.kkmids=data.deviceids;if(data.deviceids.length>0)this.currentkkm=getcurKKMFromLocalStorage();})
		.catch(err => {this.showSearchKKM=false;this.showAlertMessage=true;this.errormsg=err;console.log(err);});
	},
	savecurkkm: function(id) {
		setcurKKMToLocalStorage(id);
	},
	runCommand: function(){
		this.kkmdata='';this.kkmerr='';
		if(this.command!='' && this.currentkkm!=''){
			this.showSearchKKM=true;
			fetch("/api/run/"+this.currentkkm+"/"+this.command+"?params[0]=30",
				{
				  method: "POST", // POST, PUT, DELETE, etc.
				  headers: {
					"Content-Type": "application/json;charset=UTF-8"
				  },
				  mode: 'cors',
				  //body: JSON.stringify({params:[30]}), // undefined, string, FormData, Blob, BufferSource или URLSearchParams
				  cache: "no-store", // no-store, reload, no-cache, force-cache или only-if-cached
				}
			)
			.then(response => response.json())
			.then(data => {this.showSearchKKM=false;if(data.error){this.errormsg=data.message;this.isError=true;this.showAlertMessage=true;return;}this.retdata=data.retdata+'\n'+data.resdescr;this.kkmerr=data.kkmerr;})
			.catch(err => {this.showSearchKKM=false;this.showAlertMessage=true;this.errormsg=err;console.log(err);});
		}
		
	},
	kkmsdata: {},
	kkmdata: {
		name: "new kkm",
		adminpassword: 30,
		codepage: "cp1251",
		deviceid: "4ff2d011-898d-41c1-9bb4-777b8f69b60a",
		kkmparam: {kkmserialnum: "12345", inn: "1234567890", fname: "ООО Борей"},
		maxattempt: 12,
		password: 1,
		portconf: {name: "/dev/ttyUSB0", baud: 115200, readtimeout: 50, size: 8, parity: 0, stopbits: 1,startbits: 1},
		timeout: 0,
	},
	kkmids: [],
	getkkmdata(id) {
		this.kkmdata=this.kkmsdata[id];
		return this.kkmsdata[id];
	},
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
