package main

import (
	"encoding/binary"
	//"errors"
	"flag"
	"kkm-shtrih/drv"
	"log"
	"strconv"
	"time"

	"github.com/google/uuid"

	"net/http"

	"github.com/gin-gonic/gin"
	bolt "go.etcd.io/bbolt"
)

//DB указатель на базу данных
var DB *bolt.DB

//Kkm экземпляр драйвера
var Kkm drv.KkmDrv

//KkmServ экземпляр сервера
var KkmServ = Serv{}

//ADMINPASSWORD значение по умолчанию пароль администратора
var ADMINPASSWORD = []byte{0x1e, 0x00, 0x00, 0x00}

//DEFAULTPASSWORD значение по умолчанию для пароля кассира
var DEFAULTPASSWORD = []byte{0x1, 0x0, 0x0, 0x0}

//DEFAULTPORT порт по умолчанию
var DEFAULTPORT = "/dev/ttyUSB0"

//DEFAULTBOD скорость порта по умолчанию
var DEFAULTBOD int64 = 115200

//CODEPAGE Кодировка текста для устройств
var CODEPAGE = "cp1251"

//MAXATTEMPT Кол-во попыток и таймаут
var MAXATTEMPT int64 = 12

//BYTETIMEOUT задержка для получения одного байта порта
var BYTETIMEOUT int64 = 50 //milsec
//PORTTIMEOUT задержка для ккм
var PORTTIMEOUT int64 = 5000 //milsec, 5sec

// itob returns an 8-byte little endian representation of v.
func itob(v int64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(v))
	return b
}

// btoi returns an 8-byte little endian representation of v.
func btoi(v []byte) int64 {
	b := binary.LittleEndian.Uint64(v)
	return int64(b)
}

func runCommand(c *gin.Context) {
	hdata := make(map[string]interface{})
	deviceID := c.Param("DeviceID")
	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}
	cmd := c.Param("command")
	if cmd == "" {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "нет комманды"})
		return
	}

	strparams := c.QueryMap("params")
	params := make([]byte, 0, 64)
	//первым идет пароль, 4 байта, потом все остальные по-порядку
	for i := 0; i < len(strparams); i++ {
		if v, ok := strparams[strconv.FormatInt(int64(i), 10)]; ok {
			vint, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"error": true, "message": "параметры комманды не верны"})
				return
			}
			res := itob(vint)
			if i == 0 {
				params = append(params, res[:4]...)
			} else {
				params = append(params, res[0])
			}
		}

	}
	/*
		var icmd int64
		if cmd[0] == byte('0') && cmd[1] == byte('x') {
			icmd, err = strconv.ParseInt(cmd, 0, 10)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"error": true, "message": "плохая комманда"})
				return
			}
			_, err = kkm.Connect()
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"error": true, "message": err.Error()})
				return
			}
			kkmerr, data, err := kkm.SendCommand((uint16)(icmd), params)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"error": true, "message": err.Error()})
				return
			}
			sdata := make([]string, len(data))
			for i := 0; i < len(data); i++ {
				sdata[i] = strconv.FormatUint(uint64(data[i]), 10)
			}
			hdata["deviceID"] = deviceID
			hdata["kkmerr"] = kkmerr
			hdata["retdata"] = sdata
			hdata["error"] = false
			hdata["message"] = "ok"
			c.JSON(http.StatusOK, hdata)
			return
		}
	*/
	_, err = kkm.Connect()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": err.Error()})
		return
	}
	kkmerr, data, descr, err := kkmRunFunction(kkm, cmd, params)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": err.Error()})
		return
	}
	sdata := make([]string, len(data))
	for i := 0; i < len(data); i++ {
		sdata[i] = strconv.FormatUint(uint64(data[i]), 10)
	}
	hdata["deviceID"] = deviceID
	hdata["kkmerr"] = kkmerr
	hdata["retdata"] = sdata
	hdata["resdescr"] = descr
	hdata["error"] = false
	c.JSON(http.StatusOK, hdata)

}

func getDataKKT(c *gin.Context) {
	type TableParametersKKT struct {
		KKTNumber       string `xml:"kktnumber" binding:"-"`              //Регистрационный номер ККТ
		KKTSerialNumber string `xml:"kktserialnumber" binding:"required"` //Заводской номер ККТ
		FirmwareVersion string `xml:"FirmwareVersion" binding:"-"`        //Версия прошивки
		Fiscal          bool   `xml:"Fiscal" binding:"required"`          //Признак регистрации фискального накопителя
		FFDVersionFN    string `xml:"FFDVersionFN" binding:"required"`    //Версия ФФД ФН (одно из следующих значений "1.0","1.1")
		FFDVersionKKT   string `xml:"FFDVersionKKT" binding:"required"`   //Версия ФФД ККТ (одно из следующих значений "1.0","1.0.5","1.1")
		FNSerialNumber  string `xml:"FNSerialNumber" binding:"required"`  //Заводской номер ФН
		DocumentNumber  string `xml:"DocumentNumber" binding:"-"`         //Номер документа регистрация фискального накопителя
		DateTime        string `xml:"DateTime" binding:"-"`               //Дата и время операции регистрации фискального накопителя
		CompanyName     string `xml:"CompanyName" binding:"-"`            //Название организации
		INN             string `xml:"INN" binding:"-"`                    //ИНН организация
		SaleAddress     string `xml:"SaleAddress" binding:"-"`            //Адрес проведения расчетов
		SaleLocation    string `xml:"SaleLocation" binding:"-"`           //Место проведения расчетов
		TaxationSystems string `xml:"TaxationSystems" binding:"-"`        //Коды системы налогообложения через разделитель ",".
		//Коды системы налогообложения приведены в таблице "Системы налогообложения".
		IsOffline   bool   `xml:"IsOffline" binding:"-"`   //Признак автономного режима
		IsEncrypted bool   `xml:"IsEncrypted" binding:"-"` //Признак шифрование данных
		IsService   bool   `xml:"IsService" binding:"-"`   //Признак расчетов за услуги
		IsExcisable bool   `xml:"IsExcisable" binding:"-"` //Продажа подакцизного товара
		IsGambling  bool   `xml:"IsGambling" binding:"-"`  //Признак проведения азартных игр
		IsLottery   bool   `xml:"IsLottery" binding:"-"`   //Признак проведения лотереи
		AgentTypes  string `xml:"AgentTypes" binding:"-"`  //Коды признаков агента через разделитель ",".
		//Коды приведены в таблице 10 форматов фискальных данных.
		BSOSing            bool   `xml:"BSOSing" binding:"-"`            //Признак формирования АС БСО
		IsOnlineOnly       bool   `xml:"IsOnlineOnly" binding:"-"`       //Признак ККТ для расчетов только в Интернет
		IsAutomaticPrinter bool   `xml:"IsAutomaticPrinter" binding:"-"` //Признак установки принтера в автомате
		IsAutomatic        bool   `xml:"IsAutomatic" binding:"-"`        //Признак автоматического режима
		AutomaticNumber    string `xml:"AutomaticNumber" binding:"-"`    //Номер автомата для автоматического режима
		OFDCompany         string `xml:"OFDCompany" binding:"-"`         //Название организации ОФД
		OFDCompanyINN      string `xml:"OFDCompanyINN" binding:"-"`      //ИНН организации ОФД
		FNSURL             string `xml:"FNSURL" binding:"-"`             //Адрес сайта уполномоченного органа (ФНС) в сети «Интернет»
		SenderEmail        string `xml:"SenderEmail" binding:"-"`
	}
	var res TableParametersKKT

	deviceID := c.Param("DeviceID")

	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.XML(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}
	//запрос состояния ккм
	errcode, data, err := kkm.SendCommand(0x11, kkm.AdminPassword[:])
	if err != nil {
		c.XML(http.StatusOK, gin.H{"error": true, "message": err.Error()})
		return
	}
	if errcode > 0 {
		c.XML(http.StatusOK, gin.H{"error": true, "message": kkm.ErrState(errcode)})
		return
	}
	res.KKTSerialNumber = string(data[22:26])
	res.KKTNumber = kkm.Param.KKMNumber
	res.FirmwareVersion = string(data[1:3])
	res.FFDVersionKKT = string(data[3:5])
	res.CompanyName = kkm.Param.Fname
	res.INN = kkm.Param.Inn
	//res.DateTime = string(data[6:9])
	//res.DocumentNumber = string(data[10:12])
	//res.DateTime = string(data[15:18])
	if len(data) > 48 {

	}
	//итоги фискализации
	errcode, data, err = kkm.SendCommand(0xFF09, kkm.AdminPassword[:])
	if errcode > 0 {
		c.XML(http.StatusOK, gin.H{"error": true, "message": kkm.ErrState(errcode)})
		return
	}
	var mask byte
	/*
			Состояние фазы жизни: 1 байт
		Бит 0 – проведена настройка ФН
		Бит 1 – открыт фискальный режим
		Бит 2 – закрыт фискальный режим
		Бит 3 – закончена передача фискальных данных в ОФД
	*/
	res.Fiscal = (data[0]&0b0010) > 0 && ((data[0] & 0b0100) == 0)
	res.DateTime = string(data[:5])
	res.INN = string(data[5:17])
	res.KKTNumber = string(data[17:38])
	/*
			0	Общая
		1	Упрощенная (Доход)
		2	Упрощенная (Доход минус Расход)
		3	Единый налог на вмененный доход
		4	Единый сельскохозяйственный налог
		5	Патентная система налогообложения */
	tax := ""
	comma := ""
	mask = 0b000001
	for i := 0; i < 6; i++ {
		if data[38]&mask > 0 {
			tax = tax + comma + strconv.FormatInt(int64(i), 10)
			comma = ","
		}
		mask = mask << 1
	}
	res.TaxationSystems = tax
	/*
			Бит 0 – Шифрование
		Бит 1 – Автономный режим
		Бит 2 – Автоматический режим
		Бит 3 – Применение в сфере услуг
		Бит 4 – Режим БСО
		Бит 5 – Применение в Интернет
	*/
	res.BSOSing = data[39]&0b0010000 > 0
	res.IsOnlineOnly = data[39]&0b0100000 > 0
	res.IsAutomatic = data[39]&0b0100 > 0
	res.DocumentNumber = string(data[40:44])
	if len(data) > 53 {
		res.DocumentNumber = string(data[57:60])
		//res.DateTime = string(data[60:])
		res.OFDCompanyINN = string(data[41:53])
	}
	//запрос номера ФН
	errcode, data, err = kkm.SendCommand(0xFF02, kkm.AdminPassword[:])
	if errcode > 0 {
		c.XML(http.StatusOK, gin.H{"error": true, "message": kkm.ErrState(errcode)})
		return
	}
	res.FNSerialNumber = string(data)
	//Запрос версии ФН
	errcode, data, err = kkm.SendCommand(0xFF04, kkm.AdminPassword[:])
	if errcode > 0 {
		c.XML(http.StatusOK, gin.H{"error": true, "message": kkm.ErrState(errcode)})
		return
	}
	//Строка версии программного обеспечения ФН:16 байт ASCII
	//Тип программного обеспечения ФН: 1 байт
	//0 – отладочная версия
	//1 – серийная версия
	res.FFDVersionFN = string(data[:16])
	c.XML(http.StatusOK, res)
	return
}

func openShift(c *gin.Context) {
	type InputParameters struct {
		CashierName  string `xml:"CashierName" binding:"required"` //ФИО и должность уполномоченного лица для проведения операции
		CashierINN   string `xml:"CashierINN" binding:"-"`         //ИНН уполномоченного лица для проведения операции
		SaleAddress  string `xml:"SaleAddress" binding:"-"`        //Адрес проведения расчетов
		SaleLocation string `xml:"SaleLocation" binding:"-"`       //Место проведения расчетов
	}
	type OperationCounters struct {
		CheckCount                  int     `xml:"CheckCount" binding:"required"`                  //Количество чеков по операции данного типа
		TotalChecksAmount           float64 `xml:"TotalChecksAmount" binding:"required"`           //Итоговая сумма чеков по операциям данного типа
		CorrectionCheckCount        int     `xml:"CorrectionCheckCount" binding:"required"`        //Количество чеков коррекции по операции данного типа
		TotalCorrectionChecksAmount float64 `xml:"TotalCorrectionChecksAmount" binding:"required"` //Итоговая сумма чеков коррекции по операциям данного типа
	}
	type OutputParameters struct {
		ShiftNumber             int    `xml:"ShiftNumber" binding:"required"`      //Номер открытой смены/Номер закрытой смены
		CheckNumber             int    `xml:"CheckNumber" binding:"-"`             //Номер последнего фискального документа
		ShiftClosingCheckNumber int    `xml:"ShiftClosingCheckNumber" binding:"-"` //Номер последнего чека за смену
		DateTime                string `xml:"DateTime" binding:"required"`         //Дата и время формирования фискального документа
		ShiftState              int    `xml:"ShiftState" binding:"required"`       //Состояние смены
		//1 - Закрыта
		//2 - Открыта
		//3 - Истекла
		CountersOperationType1 OperationCounters `xml:"CountersOperationType1" binding:"-"` //Счетчики операций по типу "приход"
		//(код 1, Таблица 25 документа ФФД)
		CountersOperationType2 OperationCounters `xml:"CountersOperationType2" binding:"-"` //Счетчики операций по типу "возврат прихода"
		//(код 2, Таблица 25 документа ФФД)
		CountersOperationType3 OperationCounters `xml:"CountersOperationType3" binding:"-"` //Счетчики операций по типу "расход"
		//(код 3, Таблица 25 документа ФФД)
		CountersOperationType4 OperationCounters `xml:"CountersOperationType4" binding:"-"` //Счетчики операций по типу "возврат расхода"
		//(код 4, Таблица 25 документа ФФД)
		CashBalance                  float64 `xml:"CashBalance" binding:"-"`                  //Остаток наличных денежных средств в кассе
		BacklogDocumentsCounter      int     `xml:"BacklogDocumentsCounter" binding:"-"`      //Количество непереданных документов
		BacklogDocumentFirstNumber   int     `xml:"BacklogDocumentFirstNumber" binding:"-"`   //Номер первого непереданного документа
		BacklogDocumentFirstDateTime string  `xml:"BacklogDocumentFirstDateTime" binding:"-"` //Дата и время первого из непереданных документов
		FNError                      bool    `xml:"FNError" binding:"required"`               //Признак необходимости срочной замены ФН
		FNOverflow                   bool    `xml:"FNOverflow" binding:"required"`            //Признак переполнения памяти ФН
		FNFail                       bool    `xml:"FNFail" binding:"required"`                //Признак исчерпания ресурса ФН
	}
	var inp InputParameters
	deviceID := c.Param("DeviceID")

	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.XML(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}
	if err = c.ShouldBindXML(&inp); err != nil {
		c.XML(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err = kkmOpenShift(kkm)
	if err != nil {
		c.XML(http.StatusOK, gin.H{"error": true, "message": err.Error()})
		return
	}
	//заполним выходные параметры по запросу

}

func getServSetting(c *gin.Context) {
	hdata := make(map[string]interface{})
	var kkms = make([]string, 0, 8)
	//заполним Kkmserv из базы
	err := KkmServ.ReadServ()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": err.Error()})
		return
	}
	//прочитаем настройки сервера
	for k, v := range KkmServ.Drv {
		kkms = append(kkms, k)
		hdata[k] = v.GetStruct()
	}
	hdata["deviceids"] = kkms
	hdata["error"] = false
	c.JSON(http.StatusOK, hdata)
}

func getParamKKT(c *gin.Context) {
	hdata := make(map[string]interface{})
	deviceID := c.Param("DeviceID")
	kkm, err := KkmServ.GetDrv(deviceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": "deviceID не зарегистрирован"})
		return
	}
	//заполним Kkmserv из базы
	err = KkmServ.ReadDrvServ(deviceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": err.Error()})
		return
	}
	hdata[deviceID] = kkm.GetStruct()
	c.JSON(http.StatusOK, hdata)
}

//setServSetting установка параметров сервера
func setServSetting(c *gin.Context) {
	//hdata := make(map[string]interface{})
	//заполним Kkmserv из базы
	var jkkm drv.KkmDrvSer
	if err := c.ShouldBindJSON(&jkkm); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "error": true, "message": "bad request " + err.Error()})
		return
	}

	err := KkmServ.SetServ(&jkkm)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": true, "message": err.Error()})
		return
	}
	getServSetting(c)
	//hdata["device"] = jkkm
	//hdata["error"] = false
	//c.JSON(http.StatusOK, hdata)
}

//indexPage начальная страница приложения
func indexPage(c *gin.Context) {
	hdata := make(map[string]interface{})
	//заполним Kkmserv из базы

	hdata["page"] = "index"

	c.HTML(
		// Зададим HTTP статус 200 (OK)
		http.StatusOK,
		// Используем шаблон index.html
		"index.html",
		// Передадим данные в шаблон
		hdata,
	)
}

//editPage редактирование параметров ккм
func editPage(c *gin.Context) {
	hdata := make(map[string]interface{})
	//заполним Kkmserv из базы

	hdata["page"] = "edit"

	c.HTML(
		// Зададим HTTP статус 200 (OK)
		http.StatusOK,
		// Используем шаблон index.html
		"edit.html",
		// Передадим данные в шаблон
		hdata,
	)
}

//commandPage редактирование параметров ккм
func commandPage(c *gin.Context) {
	hdata := make(map[string]interface{})
	//заполним Kkmserv из базы

	hdata["page"] = "command"

	c.HTML(
		// Зададим HTTP статус 200 (OK)
		http.StatusOK,
		// Используем шаблон index.html
		"command.html",
		// Передадим данные в шаблон
		hdata,
	)
}

//initDB в пустой базе создает и записывает значения по умолчанию
func initDB() error {
	return DB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket([]byte("DefaultConfig"))
		if err != nil {
			return err
		}
		//var kkm = drv.KkmDrv{}
		/*
			copy(kkm.AdminPassword[:], ADMINPASSWORD)
			copy(kkm.Password[:], DEFAULTPASSWORD)
			kkm.MaxAttemp = MAXATTEMPT
			kkm.TimeOut = PORTTIMEOUT
			kkm.Opt.Baud = int(DEFAULTBOD)
			kkm.Opt.Name = DEFAULTPORT
			kkm.Opt.ReadTimeout = time.Duration(BYTETIMEOUT) * time.Millisecond
		*/
		err = b.Put([]byte("AdminPassword"), ADMINPASSWORD)
		err = b.Put([]byte("DefaultPassword"), DEFAULTPASSWORD)
		err = b.Put([]byte("DefaultPort"), []byte(DEFAULTPORT))
		err = b.Put([]byte("DefaultBod"), itob((DEFAULTBOD)))
		err = b.Put([]byte("MaxAttempt"), itob(MAXATTEMPT))

		b, err = tx.CreateBucket([]byte("Drivers"))
		uuidWithHyphen := uuid.New()
		kkm := KkmServ.New(uuidWithHyphen.String())
		v, err := kkm.Serialize()
		if err != nil {
			return err
		}
		//uuid := strings.Replace(uuidWithHyphen.String(), "-", "", -1)
		err = b.Put([]byte(kkm.DeviceID), v)
		return nil
	})
}

func main() {
	// открываем базу и считываем конфиг
	// Open the my.db data file in your current directory.
	// It will be created if it doesn't exist
	//port := flag.String("port", "3000", "Номер порта")
	port := flag.Int("port", 3000, "Номер порта")
	portstr := ":" + strconv.Itoa(*port)
	flag.Parse()
	//portstr := ":" + strconv.Itoa(*port)
	var err error
	DB, err = bolt.Open("kkm.db", 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatal(err)

	}
	defer DB.Close()

	err = initDB()
	if err != nil && err != bolt.ErrBucketExists {
		log.Fatal(err)
	}

	//init KkmServ
	err = KkmServ.ReadServ()
	if err != nil {
		log.Fatal(err)
	}

	router := gin.Default()
	router.Static("/assets", "./assets")
	router.LoadHTMLGlob("./tpl/*")
	//начальная страница настройки
	router.GET("/", indexPage)
	router.GET("/edit", editPage)
	router.GET("/command", commandPage)
	api := router.Group("/api/")
	{
		api.GET("GetServSetting/", getServSetting)
		api.PUT("SetServSetting/", setServSetting)
		api.POST("run/:DeviceID/:command", runCommand)
		api.GET("GetParamKKT/:DeviceID", getParamKKT)
		api.POST("GetDataKKT/:DeviceID", getDataKKT)
		//api.POST("OperationFN/", OperationFN)
		api.POST("OpenShift/:DeviceID", openShift)
		//api.POST("CloseShift/", CloseShift)
		//api.POST("ProcessCheck/", ProcessCheck)
		//api.POST("ProcessCorrectionCheck/", ProcessCorrectionCheck)
		//api.POST("PrintTextDocument/", PrintTextDocument)
		//api.POST("CashInOutcome/", CashInOutcome)
		//api.POST("PrintXReport/", PrintXReport)
		//api.POST("PrintCheckCopy/", PrintCheckCopy)
		//api.POST("GetCurrentStatus/", GetCurrentStatus)
		//api.POST("ReportCurrentStatusOfSettlements/", ReportCurrentStatusOfSettlements)
		//api.POST("OpenCashDrawer/", OpenCashDrawer)
		//api.POST("GetLineLength/", GetLineLength)
		//api.POST("CashInOutcome/", CashInOutcome)
		//api.DELETE("stores/", deleteStocks)
		//api.DELETE("goods/:id", DeleteProduct)
	}
	router.Run(portstr)
}
