package drv

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/tarm/serial"

	// bolt "go.etcd.io/bbolt"
	"golang.org/x/text/encoding/charmap"
)

//ENQ команда ККМ для перевода в режим ожидания команды
const ENQ = 0x05

//STX команда от ККМ указывает что следом идут данные
const STX = 0x02

//ACK команда ККМ означает все ОК, принял
const ACK = 0x06

//NAK команда ККМ означает что была ошибка приема
const NAK = 0x15

//todo передать это в переменные сервера ->

//MaxAttempBusy максимальное число попыток ожидания занятости ккм
const MaxAttempBusy = 10

//TimeWhileBusy время между попвтками
const TimeWhileBusy = 1000

//MaxTimeKKMBusy время после которого ккм автоматически будет освобождена, сек
const MaxTimeKKMBusy = 60

//Digit разрядность денежных величин ккм
const Digit = 2

//<-todo

//KkmDrv структура драйвера
type KkmDrv struct {
	mu            sync.RWMutex
	Name          string
	DeviceID      string
	Port          *serial.Port
	Opt           serial.Config
	TimeOut       int64
	Connected     bool
	Password      [4]byte
	AdminPassword [4]byte
	MaxAttemp     int64
	CodePage      string
	Param         KkmParam
	State         KkmState
	FNState       KkmFNState
}

//KkmParam параметры модели, серийный номер, ИНН и пр
type KkmParam struct {
	KKMRegNumber    string `json:"kkmregnum"`    //Регистрационный номер
	KKMSerialNumber string `json:"kkmserialnum"` //Заводской номер
	Inn             string `json:"inn"`
	Fname           string `json:"fname"`
	RNM             string `json:"rnm"` //РНМ
	//длина строки чека
	LenLine uint8 `json:"lenline"`
}

//PortConf копия конфигурации порта для сериализации
type PortConf struct {
	Name        string `json:"name"`
	Baud        int    `json:"baud"`
	ReadTimeout int    `json:"readtimeout"` // Total timeout  time.Duration
	// Size is the number of data bits. If 0, DefaultSize is used.
	Size byte `json:"size"`
	// Parity is the bit to use and defaults to ParityNone (no parity bit).
	Parity byte `json:"parity"`
	// Number of stop bits to use. Default is 1 (1 stop bit).
	StopBits byte `json:"stopbits"`
}

//KkmDrvSer копия конфигурации ккм для сериализации
type KkmDrvSer struct {
	Name          string   `json:"name"`
	DeviceID      string   `json:"deviceid"`
	Opt           PortConf `json:"portconf"`
	TimeOut       int64    `json:"timeout"`
	Password      int64    `json:"password"`
	AdminPassword int64    `json:"adminpassword"`
	MaxAttemp     int64    `json:"maxattempt"`
	CodePage      string   `json:"codepage"`
	Param         KkmParam `json:"kkmparam"`
}

//KkmState текущее состояние ККМ
type KkmState struct {
	//Busy занята ли ккм
	Busy bool
	//ProcID занята ли ккм
	ProcID int
	//состояние ккм
	State byte
	//состояние ккм
	SubState byte
	//флаг состояния
	Flag uint16 //2 byte
	//флаг состояния ЭКЛЗ
	FlagFP byte
	//LastSession номер последней смены
	LastSession uint16
	//Err
	Err byte
}

//KkmFNState текущее состояние ФН
type KkmFNState struct {
	//Флаги предупреждения ФН Диапазон значений: 1 – Срочная замена криптографического сопроцессора (до окончания срока действия 3 дня)
	//2 – Исчерпание ресурса криптографического сопроцессора (до окончания срока действия 30 дней)
	//4 – Переполнение памяти ФН (Архив ФН заполнен на 90 %)
	//8 – Превышено время ожидания ответа ОФД
	FNWarningFlags uint8
	//Состояние жизни ФН 0x00	стадия1	Производственная стадия,0x01 стадия2 Готовность к фискализации,	0x03	стадия3	Фискальный режим, 0x07	стадия4	Фискальный режим закрыт. Передача фискальных документов в ОФД
	//0x0F	стадия5	Чтение данных из Архива ФН
	FNLifeState uint8
	//Текущий документ ФН
	FNCurrentDocument uint8
	//Данные документа ФН  Диапазон значений: 0 – нет данных документа; 1 – получены данные документа.
	FNDocumentData uint8
	//Состояние смены ФН Диапазон значений: 0 – смена закрыта; 1 – смена открыта.
	FNSessionState uint8
	//Номер документа  Диапазон значений: 1…9999.
	DocumentNumber uint32
	//Заводской номер
	SerialNumber string //Заводской номер
	//дата время фискализации (перерегистрации)
	DateTime uint64
}

//OsPort доступный порт в ОС
type OsPort struct {
	Baud   int    `json:"baud"`
	Port   string `json:"port"`
	Device string `json:"device"`
	Err    string `json:"err"`
}

/*
Пример обмена данными с ФР:
ENQ ->
<- ACK
Команда ->
<-NAK (CRC не сошелся)
Команда -> (повтор команды)
<- ACK
<- Ответ
ACK ->
*/
//decodeWindows1251 из win1251 в uft-8
func decodeWindows1251(ba []uint8) []uint8 {
	dec := charmap.Windows1251.NewDecoder()
	out, _ := dec.Bytes(ba)
	return out
}

//encodeWindows1251 из uft-8 в win1251
func encodeWindows1251(ba string) []uint8 {
	enc := charmap.Windows1251.NewEncoder()
	out, _ := enc.String(ba)
	return []uint8(out)
}

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

//money2byte преобразует числа в слайс байт для передачи в ккм
func money2byte(money float64, digits int) []byte {
	//Разрядность денежных величин
	//Все суммы в данном разделе – целые величины, указанные в «мде». МДЕ –
	//минимальная денежная единица. С 01.01.1998 в Российской Федерации 1 МДЕ равна 1
	//копейке (до 01.01.1998 1 МДЕ была равна 1 рублю).
	//Формат передачи значений
	//Все числовые величины передаются в двоичном формате, если не указано другое.
	//Первым передается самый младший байт, последним самый старший байт.
	//При передаче даты (3 байта) сначала передаётся число (1 байт – ДД), затем месяц (2
	//байта – ММ), и последним – год (1 байт – ГГ).
	//При передаче времени (3 байта) первым байтом передаются часы (1 байт – ЧЧ), затем
	//минуты (1 байт – ММ), и последними передаются секунды (1 байт – СС).
	v := int64(math.Round(money * math.Pow10(digits))) //12.436*100=1243.6 = 1244 = int64(1244)
	return itob(v)
}

//q2byte преобразует количкство в слайс байт для передачи в ккм
func q2byte(q float64) []byte {
	//округляем до 3-х знаков
	v := int64(math.Round(q * 1000)) //12.43675*1000=12436.75 = 12437 = int64(12437)
	return itob(v)
}

//GetPortName вернет список портов
func GetPortName() ([]OsPort, string) {
	//где мы? windows linux darwin?
	goos := runtime.GOOS
	ports := make([]string, 256)
	bauds := []int{2400, 4800, 9600, 19200, 38400, 57600, 230400, 460800, 921600}
	switch goos {
	case "linux":
		for i := 0; i < 128; i++ {
			ports[i] = "/dev/ttyS" + strconv.FormatInt(int64(i), 10)
		}
		for i := 0; i < 128; i++ {
			ports[i] = "/dev/ttyUSB" + strconv.FormatInt(int64(i), 10)
		}
	case "windows":
		for i := 0; i < 256; i++ {
			ports[i] = "com" + strconv.FormatInt(int64(i), 10)
		}
	}
	//попробуем найти действующие порты
	confok := make([]OsPort, 0, 256)
	var cfg OsPort
	for i := 0; i < 256; i++ {
		//for b := 0; b < len(bauds); b++ {
		c := &serial.Config{Name: ports[i], Baud: 115200, ReadTimeout: time.Second * 5}
		prt, err := serial.OpenPort(c)
		if err == nil {
			cfg = OsPort{}
			cfg.Baud = 115200
			cfg.Port = ports[i]
			confok = append(confok, cfg)
			prt.Close()
			for b := 0; b < len(bauds); b++ {
				cfg.Baud = bauds[b]
				cfg.Port = ports[i]
				confok = append(confok, cfg)
			}
		} else {
			if err.Error() == "Access denided" {
				cfg = OsPort{}
				cfg.Baud = 115200
				cfg.Port = ports[i]
				confok = append(confok, cfg)
				for b := 0; b < len(bauds); b++ {
					cfg.Baud = bauds[b]
					cfg.Port = ports[i]
					confok = append(confok, cfg)
				}
			}
		}
		//}
	}
	return confok, goos
}

//SearchKKM поиск ккм
func SearchKKM() []OsPort {
	//попробуем найти действующие порты
	retok := make([]OsPort, 0, 256)
	//var cfg conf
	k := new(KkmDrv)
	k.MaxAttemp = 3
	k.Connected = false
	k.TimeOut = 8
	k.TimeOut = 2000
	portconf, _ := GetPortName()
	founded := make(map[string]bool)
	//для начала переберем порты на скорости 115200
	set := make(map[string]bool) // New empty set
	for _, cfg := range portconf {
		set[cfg.Port] = true
	}
	for p := range set { // Loop
		c := &serial.Config{Name: p, Baud: 115200, ReadTimeout: time.Second * 1}
		k.SetConfig(*c)
		errcode, data, err := k.SendCommand(0xfc, []byte{})
		if err == nil {
			r := OsPort{}
			r.Baud = 115200
			r.Port = p
			if errcode == 0 {
				r.Device = string(decodeWindows1251(data[6:]))
				r.Err = ""
				founded[p] = true
			} else {
				r.Err = k.ParseErrState(errcode)
			}
			retok = append(retok, r)
		}
		k.Close()
	}

	for _, cfg := range portconf {
		if !founded[cfg.Port] {
			c := &serial.Config{Name: cfg.Port, Baud: cfg.Baud, ReadTimeout: time.Second * 2}
			k.SetConfig(*c)
			errcode, data, err := k.SendCommand(0xfc, []byte{})
			if err == nil {
				r := OsPort{}
				r.Baud = cfg.Baud
				r.Port = cfg.Port
				if errcode == 0 {
					r.Device = string(decodeWindows1251(data[6:]))
					r.Err = ""
					founded[cfg.Port] = true
				} else {
					r.Err = k.ParseErrState(errcode)
				}
				retok = append(retok, r)
			}
			k.Close()
		}
	}
	return retok
}

//PrintString печатает строку чека
func (kkm *KkmDrv) PrintString(pass []byte, str string) (byte, error) {
	//Пароль оператора (4 байта)
	//Флаги (1 байт) Бит 0 – контрольная лента, Бит 1 – чековая лента, Бит 2–подкладной документ, Бит 3– слип-чек, Бит 6– перенос строк, Бит 7–отложенная печать
	//Печатаемые символы6,7,8,9,10 (40 или X байт)
	tabparam := make([]byte, 45)
	copy(tabparam, pass[:4])
	tabparam[4] = 1 //контрольная лента
	copy(tabparam[5:], encodeWindows1251(string(str)))
	errcode, _, err := kkm.SendCommand(0x17, tabparam)
	return errcode, err
}

//ContinuePrint продолжить печать
func (kkm *KkmDrv) ContinuePrint(pass []byte) (byte, error) {
	//Продолжение печати
	if len(pass) == 0 {
		pass = kkm.GetAdminPass()
	}
	errcode, _, err := kkm.SendCommand(0xB0, pass)
	return errcode, err
}

//CancelCheck отменяет чек
func (kkm *KkmDrv) CancelCheck(pass []byte) (byte, error) {
	if len(pass) == 0 {
		pass = kkm.GetAdminPass()
	}
	errcode, _, err := kkm.SendCommand(0x88, pass)
	return errcode, err
}

//OpenCheck открывает чек
func (kkm *KkmDrv) OpenCheck(pass []byte, chktype byte) (byte, error) {
	if len(pass) == 0 {
		pass = kkm.GetAdminPass()
	}
	//Открыть чек
	//Команда: 8DH. Длина сообщения: 6 байт.
	//Пароль оператора (4 байта) Тип документа (1 байт):
	//«0» – продажа  «1» – покупка  «2» – возврат продажи  «3» – возврат покупки  Код ошибки (1 байт) Порядковый номер оператора (1 байт) 1…30
	tabparam := make([]byte, 5)
	copy(tabparam, pass[:4])
	tabparam[4] = chktype
	//open chk
	errcode, _, err := kkm.SendCommand(0x8d, tabparam)
	return errcode, err
}

//CloseCheck закрывает чек
func (kkm *KkmDrv) CloseCheck(pass []byte, summa map[int]float64, tax map[string]float64, taxsystem, rnd byte, printstring string) (retsum float64, chknum int, fiscalsign string, dtime string, errcode byte, err error) {
	//taxsystem = Код системы налогообложения.
	//0	Общая
	//1	Упрощенная (Доход)
	//2	Упрощенная (Доход минус Расход)
	//3	Единый налог на вмененный доход
	//4	Единый сельскохозяйственный налог
	//5	Патентная система налогообложения
	//rnd округление до рубля в копейках
	/*
		Закрытие чека расширенное
		Команда: FF45H. Длина сообщения: 182 байт.
		Пароль системного администратора: 4 байта
		Сумма наличных (5 байт) [4:]
		Сумма типа оплаты 2 (5 байт) [9:]
		Сумма типа оплаты 3 (5 байт) [14:]
		Сумма типа оплаты 4 (5 байт) [19:]
		Сумма типа оплаты 5 (5 байт) [24:]
		Cумма типа оплаты 6 (5 байт) [29:]
		Сумма типа оплаты 7 (5 байт) [34:]
		Сумма типа оплаты 8 (5 байт) [39:]
		Сумма типа оплаты 9 (5 байт) [44:]
		Сумма типа оплаты 10 (5 байт) [49:]
		Сумма типа оплаты 11 (5 байт) [54:]
		Сумма типа оплаты 12 (5 байт) [59:]
		Сумма типа оплаты 13 (5 байт) [64:]
		Сумма типа оплаты 14 (5 байт) (предоплата) [69:]
		Сумма типа оплаты 15 (5 байт) (постоплата) [74:]
		Сумма типа оплаты 16 (5 байт) (встречное представление) [79:]
		Округление до рубля в копейках (1 байт) [84]
		Налог 1 (5 байт) (НДС 18%)	[85:]
		Налог 2 (5 байт) (НДС 10%)  [90:]
		Оборот по налогу 3 (5 байт) (НДС 0%) [95:]
		Оборот по налогу 4 (5 байт) (Без НДС) [100:]
		Налог 5 (5 байт) (НДС расч. 18/118)  [105:]
		Налог 6 (5 байт) (НДС расч. 10/110)  [110:]
		Система налогообложения ( 1 байт)1   [115]
			Бит 0 – ОСН
			Бит 1 – УСН доход
			Бит 2 – УСН доход минус расход
			Бит 3 – ЕНВД
			Бит 4 – ЕСП
			Бит 5 – ПСН
			Текст (0-64 байт)  [116:180]
		Ответ: FF45h Длина сообщения: 14 (19) байт
		Код ошибки: 1 байт
		Сдача: 5 байт [:5]
		Номер ФД :4 байта [5:9]
		Фискальный признак: 4 байта  [9:13]
		Дата и время: 5 байт DATE_TIME [13:18]<-может остутствовать
	*/
	//В соответствие с п.5 к табл. 19 ФФД делается проверка параметра "Округление до рубля в копейках (1 байт)" в команде FF45H: подытог чека (сумма тегов 1043) в рублях должен быть равен тегу 1020 в рублях (1020 формирует ФН из принятых тегов 1031+1081+1215+1216+1217).
	cash := summa[1]
	electronicPayment := summa[2]
	prePayment := summa[14]
	postPayment := summa[15]
	barter := summa[16]
	param := make([]byte, 180)
	copy(param, pass)
	if cash > 0 {
		copy(param[4:], money2byte(cash, Digit))
	}
	if electronicPayment > 0 {
		copy(param[9:], money2byte(electronicPayment, Digit))
	}
	if prePayment > 0 {
		copy(param[69:], money2byte(prePayment, Digit))
	}
	if postPayment > 0 {
		copy(param[74:], money2byte(postPayment, Digit))
	}
	if barter > 0 {
		copy(param[79:], money2byte(barter, Digit))
	}
	param[84] = rnd
	//Налог 1=НДС 18%,	Налог 2 =НДС 10%,налог 3 =НДС 0%,налог 4 =(Без НДС),Налог 5 = 18/118,	Налог 6 = (НДС расч. 10/110)
	//"none","20","18","10","0","20/120","18/118","10/110"
	if tax["20"] > 0 || tax["18"] > 0 || tax["1"] > 0 {
		copy(param[85:], money2byte(tax["20"]+tax["18"]+tax["1"], Digit))
	}
	if tax["10"] > 0 || tax["2"] > 0 {
		copy(param[90:], money2byte(tax["10"]+tax["2"], Digit))
	}
	if tax["0"] > 0 || tax["3"] > 0 {
		copy(param[95:], money2byte(tax["0"]+tax["3"], Digit))
	}
	if tax["none"] > 0 || tax["4"] > 0 {
		copy(param[100:], money2byte(tax["none"]+tax["4"], Digit))
	}
	if tax["20/120"] > 0 || tax["18/118"] > 0 || tax["5"] > 0 {
		copy(param[105:], money2byte(tax["20/120"]+tax["18/118"]+tax["5"], Digit))
	}
	if tax["10/110"] > 0 || tax["6"] > 0 {
		copy(param[110:], money2byte(tax["10/110"]+tax["6"], Digit))
	}
	param[115] = 0b00000001 << taxsystem
	if len(printstring) > 0 {
		copy(param[116:], encodeWindows1251(string(printstring))[:64])
	}
	errcode, data, err := kkm.SendCommand(0xff45, param)
	retsum = float64(btoi(data[:5]) / 100)
	chknum = int(btoi(data[5:9]))
	fiscalsign = string(decodeWindows1251(data[9:13]))
	if len(data) > 14 {
		dtime = time.Unix(btoi(data[13:]), 0).Format("2006-01-02 15:04:05")
	} else {
		dtime = time.Now().Format("2006-01-02 15:04:05")
	}
	return
}

//FNOperation Операция на ФН для печати чека
func (kkm *KkmDrv) FNOperation(pass []byte, optype int, q float64, price float64, ammount float64, taxrate float64, tax string, department int, paymentmethod int, calculationsubject int, name string) (byte, error) {
	/*optype 1 - приход денежных средств
	2 - возврат прихода денежных средств
	3 - расход денежных средств
	4 - возврат расхода денежных средств*/
	/*
		Код команды FF46h . Длина сообщения: 160 байт.
		Пароль: 4 байта [:4]
		Тип операции: 1 байт [4]
		1 – Приход,
		2 – Возврат прихода,
		3 – Расход,
		4 – Возврат расхода
		Количество: 6 байт ( 6 знаков после запятой ) [5:11]
		Цена: 5 байт	[11:16]
		Сумма операций: 5 байт  [16:21]  (- если сумма операции 0xffffffffff то сумма операции рассчитывается кассой как цена х количество,
		в противном случае сумма операции берётся из команды и не должна отличаться
		более чем на +-1 коп от рассчитанной кассой)
		Налог: 5 байт [21:26] (В режиме начисления налогов 1 (1 Таблица) налоги на позицию и на чек должны
		передаваться из верхнего ПО. Если в сумме налога на позицию передать 0xFFFFFFFFFF то
		считается что сумма налога на позицию не указана, в противном случае сумма налога
		учитывается ФР и передаётся в ОФД. Для налогов 3 и 4 сумма налога всегда считается
		равной нулю и в ОФД не передаётся)

		Налоговая ставка: 1 байт [26]
			1. НДС 18%;
			2. НДС 10%;
			3. НДС 0%;
			4. Без налога;
			5. Ставка 18/118;
			6. Ставка 10/110.
		Номер отдела: 1 байт [27]
		0…16 – режим свободной продажи, 255 – режим продажи по коду товара
		Признак способа расчёта : 1 байт [28]
		Признак предмета расчёта: 1 байт [29]
		Наименование товара: 0-128 байт ASCII [30:158]
		Если строка начинается символами // то она передаётся на сервер ОФД но не печатается на кассе
		Ответ: FF46h Длина сообщения: 1 байт.
		Код ошибки: 1 байт
	*/
	tabparam := make([]byte, 160)
	copy(tabparam, pass[:4])
	tabparam[4] = byte(optype)
	//округляем до 6-х знаков
	v := int64(math.Round(q * 1000000)) //12.43675*1000=12436.75 = 12437 = int64(12437)
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(v))
	copy(tabparam[5:], b)                            //количество 6 byte
	v = int64(math.Round(price * math.Pow10(Digit))) //12.43675*1000=12436.75 = 12437 = int64(12437)
	binary.LittleEndian.PutUint64(b, uint64(v))
	copy(tabparam[11:], b)                             //цена 5 byte
	v = int64(math.Round(ammount * math.Pow10(Digit))) //12.43675*1000=12436.75 = 12437 = int64(12437)
	binary.LittleEndian.PutUint64(b, uint64(v))
	copy(tabparam[16:], b)                             //сумма 5 byte
	v = int64(math.Round(taxrate * math.Pow10(Digit))) //12.43675*1000=12436.75 = 12437 = int64(12437)
	binary.LittleEndian.PutUint64(b, uint64(v))
	copy(tabparam[21:], b) //налог 5 byte
	switch tax {
	/* kkm: 1. НДС 18%;
	2. НДС 10%;
	3. НДС 0%;
	4. Без налога;
	5. Ставка 18/118;
	6. Ставка 10/110.*/
	case "none", "4":
		tabparam[26] = 4
	case "20":
		tabparam[26] = 1
	case "18", "1":
		tabparam[26] = 1
	case "10", "2":
		tabparam[26] = 2
	case "0", "3":
		tabparam[26] = 3
	case "20/120", "18/118", "5":
		tabparam[26] = 5
	case "10/110", "6":
		tabparam[26] = 6
	}
	tabparam[27] = byte(department)
	//(1 - предоплата 100%; 2- частичная предоплата; 3- аванс; 4- полный расчет; 5- частичный расчет, кредит..
	tabparam[28] = byte(paymentmethod)
	/*1	Товар
	2	Подакцизный товар
	3	Работа
	4	Услуга
	5	Ставка азартной игры
	6	Выигрыш азартной игры
	7	Лотерейный билет
	8	Выигрыш лотереи
	9	Предоставление РИД
	10	Платеж
	11	Агентское вознаграждение
	12	Составной предмет расчета
	13	Иной предмет расчета*/
	tabparam[29] = byte(calculationsubject)
	copy(tabparam[30:], encodeWindows1251(string(name)))
	errcode, _, err := kkm.SendCommand(0xff46, tabparam)
	return errcode, err
}

//FNSendTLV Передать произвольную TLV структуру чека tipparam="INT","STRING","DATE"
func (kkm *KkmDrv) FNSendTLV(pass []byte, teg uint16, val []byte) (uint8, error) {
	if len(val) > 250 {
		val = val[:250]
	}
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, teg)
	tlv := make([]byte, len(val)+6+2)                  // +6=pass+tag +2=len tlv
	copy(tlv, pass[:4])                                //4 byte
	copy(tlv[4:], b[:2])                               //teg 2 byte
	binary.LittleEndian.PutUint16(b, uint16(len(val))) //len 2 byte
	copy(tlv[6:], b[:2])                               //pass+tag+len
	copy(tlv[8:], val)
	errcode, _, err := kkm.SendCommand(0xff0c, tlv)
	if err != nil {
		log.Printf("fnSendTLV: %v", err)
		return 1, err
	}
	if errcode > 0 {
		return errcode, errors.New(kkm.ParseErrState(errcode))
	}
	return 0, nil
}

//FNSendTLVOperation Передать произвольную TLV структуру операции
func (kkm *KkmDrv) FNSendTLVOperation(pass []byte, teg uint16, val []byte) (uint8, error) {
	if len(val) > 250 {
		val = val[:250]
	}
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, teg)
	tlv := make([]byte, len(val)+6+2)                  // +6=pass+tag +2=len tlv
	copy(tlv, pass[:4])                                //4 byte
	copy(tlv[4:], b[:2])                               //teg 2 byte
	binary.LittleEndian.PutUint16(b, uint16(len(val))) //len 2 byte
	copy(tlv[6:], b[:2])                               //pass+tag+len
	copy(tlv[8:], val)
	errcode, _, err := kkm.SendCommand(0xff4d, tlv)
	if err != nil {
		log.Printf("fnSendTLV: %v", err)
		return 1, err
	}
	if errcode > 0 {
		return errcode, errors.New(kkm.ParseErrState(errcode))
	}
	return 0, nil
}

//Serialize преобразует данные в json byte
func (kkm *KkmDrv) Serialize() ([]byte, error) {
	return json.Marshal(kkm.GetStruct())
}

//FNGetFNState читает данные ФН
func (kkm *KkmDrv) FNGetFNState() KkmFNState {
	kkm.mu.RLock()
	f := kkm.FNState
	kkm.mu.RUnlock()
	return f
}

//ChkBusy проверяет занятость ККМ, если в течении MaxAttempt попыток не дождемся освобождения ККМ вернем true, иначе свободна вернем false
func (kkm *KkmDrv) ChkBusy(procid int) bool {
	//Если наш процесс занял ккм, то вернем false
	st := kkm.GetState()
	if st.Busy == false {
		return false
	}
	if st.ProcID == procid {
		return false
	}
	//если ккм занята давно, то освободим
	if MaxTimeKKMBusy < time.Since(time.Unix(int64(st.ProcID), 0)).Seconds() {
		kkm.SetBusy(0)
	}
	for i := 0; i < MaxAttempBusy; i++ {
		time.Sleep(time.Duration(TimeWhileBusy * int64(time.Millisecond)))
		st = kkm.GetState()
		if st.Busy == false {
			return false
		}
	}
	if st.Busy == false {
		return false
	}
	return true
}

//SetBusy установит занятость/свободно ккм
func (kkm *KkmDrv) SetBusy(procid int) {
	kkm.mu.Lock()
	if procid > 0 {
		kkm.State.Busy = true
		kkm.State.ProcID = procid
	} else {
		kkm.State.Busy = false
		kkm.State.ProcID = 0
	}
	kkm.mu.Unlock()
}

//GetConnected вернет kkm.Connected mutex-op
func (kkm *KkmDrv) GetConnected() bool {
	kkm.mu.RLock()
	st := kkm.Connected
	kkm.mu.RUnlock()
	return st
}

//SetConnected установит kkm.Connected=b mutex-op
func (kkm *KkmDrv) SetConnected(b bool) {
	kkm.mu.Lock()
	kkm.Connected = b
	kkm.mu.Unlock()
}

//GetParam вернет RNM и SerialNumber ккм mutex-op
func (kkm *KkmDrv) GetParam() KkmParam {
	kkm.mu.RLock()
	param := kkm.Param
	kkm.mu.RUnlock()
	return param
}

//SetLenLine установит длину строки
func (kkm *KkmDrv) SetLenLine(ll uint8) {
	kkm.mu.Lock()
	kkm.Param.LenLine = ll
	kkm.mu.Unlock()
}

//GetLenLine dthytn длину строки
func (kkm *KkmDrv) GetLenLine() uint8 {
	kkm.mu.RLock()
	ll := kkm.Param.LenLine
	kkm.mu.RUnlock()
	return ll
}

//SetParam установит RNM и SerialNumber ккм mutex-op
func (kkm *KkmDrv) SetParam(fname, inn, serialnum, rnm string) {
	kkm.mu.Lock()
	if fname != "-" {
		kkm.Param.Fname = fname
	}
	if inn != "-" {
		kkm.Param.Inn = inn
	}
	if serialnum != "-" {
		kkm.Param.KKMSerialNumber = serialnum
	}
	if rnm != "-" {
		kkm.Param.RNM = rnm
	}
	kkm.mu.Unlock()
}

//GetState вернет статус ккм mutex-op {busy, numState, descr}
func (kkm *KkmDrv) GetState() KkmState {
	kkm.mu.RLock()
	st := kkm.State
	kkm.mu.RUnlock()
	return st
}

//SetErrState установит статус ошибки ккм mutex-op
func (kkm *KkmDrv) SetErrState(e byte) {
	kkm.mu.Lock()
	kkm.State.Err = e
	kkm.mu.Unlock()
}

//SetState установит статус ккм (State, subState, flag, flagFP)
func (kkm *KkmDrv) SetState(state, substate byte, flag uint16, flagfp byte) {
	kkm.mu.Lock()
	kkm.State.State = state
	kkm.State.SubState = substate
	kkm.State.Flag = flag //binary.LittleEndian.Uint16(flag)
	kkm.State.FlagFP = flagfp
	kkm.mu.Unlock()
}

//GetStatus заполнит структуру ствтуса ККМ и вернет номер режима и ошибку
func (kkm *KkmDrv) GetStatus() (int, error) {
	admpass := kkm.GetAdminPass()
	errcode, data, err := kkm.SendCommand(0x11, admpass)
	if err != nil {
		return 1, err
	}
	if errcode > 0 {
		err = errors.New("Ошибка kkm: " + kkm.ParseErrState(errcode))
		return int(errcode), err
	}
	/* пароль 1 байт
	Версия ПО ККТ (2 байта) data[1:3]
	Сборка ПО ККТ (2 байта) data[3:5]
	Дата ПО ККТ (3 байта) ДД-ММ-ГГ  data[5:8]
	Номер в зале (1 байт) data[8]
	Сквозной номер текущего документа (2 байта)  data[9:11]
	Флаги ККТ (2 байта) data[11:13]
	Режим ККТ (1 байт)  data[13]
	Подрежим ККТ (1 байт) data[14]
	Порт ККТ (1 байт) data[15]
	Дата (3 байта) ДД-ММ-ГГ data[16:19]
	Время (3 байта) ЧЧ-ММ-СС data[19:22]
	Заводской номер (4 байта) младшее длинное слово 6-байтного числа (см.ниже)  uint64(binary.LittleEndian.Uint32(data[30:34]))
	Номер последней закрытой смены (2 байта) data[34:36]
	Количество перерегистраций (фискализаций) (1 байт) data[28]
	Количество оставшихся перерегистраций (фискализаций) (1 байт) data[29]
	ИНН (6 байт) data[40:46]
	Заводской номер2  data[36:38]
	*/
	kkm.SetState(data[13], data[14], binary.LittleEndian.Uint16(data[11:13]), data[29])
	kkm.mu.Lock()
	kkm.State.LastSession = binary.LittleEndian.Uint16(data[34:36])
	kkm.mu.Unlock()
	inn := make([]byte, 8)
	copy(inn, data[40:46])
	kkm.SetParam("-", strconv.FormatUint(uint64(binary.LittleEndian.Uint64(inn)), 10), strconv.FormatUint(uint64(binary.LittleEndian.Uint32(data[30:34])), 10), "-")
	state, _ := kkm.ParseState(data[13])
	return state, nil
}

//GetStatus10 заполнит структуру ствтуса ККМ и вернет номер режима и ошибку
func (kkm *KkmDrv) GetStatus10() (uint8, error) {
	admpass := kkm.GetAdminPass()
	errcode, data, err := kkm.SendCommand(0x10, admpass)
	if err != nil {
		return 1, err
	}
	if errcode > 0 {
		err = errors.New("Ошибка kkm: " + kkm.ParseErrState(errcode))
		return errcode, err
	}
	/*	data[13] Причина завершения печати или промотки бумаги:
		0 – печать завершена успешно
		1 – произошел обрыв бумаги
		2 – ошибка принтера (перегрев головки, другая ошибка)
		5 – идет печать
	*/
	st := kkm.GetState()
	kkm.SetState(data[3], data[4], binary.LittleEndian.Uint16(data[1:3]), st.FlagFP)
	return data[3], nil
}

//GetStruct преобразует данные в структуру для последующего преобазования в json byte
func (kkm *KkmDrv) GetStruct() KkmDrvSer {
	var pconf = PortConf{}
	var sr = KkmDrvSer{}
	kkm.mu.RLock()
	defer kkm.mu.RUnlock()
	pconf.Name = kkm.Opt.Name
	pconf.Baud = kkm.Opt.Baud
	pconf.Parity = byte(kkm.Opt.Parity)
	pconf.StopBits = byte(kkm.Opt.StopBits)
	pconf.Size = byte(kkm.Opt.Size)
	pconf.ReadTimeout = int(kkm.Opt.ReadTimeout / time.Millisecond)
	var param = KkmParam{}
	param.Fname = kkm.Param.Fname
	param.Inn = kkm.Param.Inn
	param.KKMSerialNumber = kkm.Param.KKMSerialNumber
	param.RNM = kkm.Param.RNM

	res := binary.LittleEndian.Uint32(kkm.AdminPassword[:])
	sr.AdminPassword = int64(res)
	sr.CodePage = kkm.CodePage
	sr.Name = kkm.Name
	sr.DeviceID = kkm.DeviceID
	sr.MaxAttemp = kkm.MaxAttemp
	sr.Password = int64(binary.LittleEndian.Uint32(kkm.Password[:]))
	sr.Opt = pconf
	sr.TimeOut = kkm.TimeOut
	sr.Param = param
	return sr
}

//SetErrState установит код ошибки
//func (kkm *KkmDrv) SetErrState(e byte) {
//	kkm.mu.Lock()
//	defer kkm.mu.Unlock()
//	kkm.State.Err = e
//}

//GetAdminPass прочитает пароль администратора
func (kkm *KkmDrv) GetAdminPass() []byte {
	kkm.mu.RLock()
	defer kkm.mu.RUnlock()
	return kkm.AdminPassword[:]
}

//GetPass прочитает пароль оператора
func (kkm *KkmDrv) GetPass() []byte {
	kkm.mu.RLock()
	defer kkm.mu.RUnlock()
	return kkm.Password[:]
}

//GetDataErrState прочитает код ошибки
func (kkm *KkmDrv) GetDataErrState() byte {
	kkm.mu.RLock()
	defer kkm.mu.RUnlock()
	return kkm.State.Err
	//strErr:=kkm.ErrState(e)
	//return int(e), strErr
}

//SetDataFromStruct заполняет данные из структуры, которая десериализуется из json byte
func (kkm *KkmDrv) SetDataFromStruct(jkkm *KkmDrvSer) {
	kkm.mu.Lock()
	defer kkm.mu.Unlock()
	kkm.Opt.Name = jkkm.Opt.Name
	kkm.Opt.Baud = jkkm.Opt.Baud
	kkm.Opt.Parity = serial.Parity(jkkm.Opt.Parity)
	kkm.Opt.StopBits = serial.StopBits(jkkm.Opt.StopBits)
	kkm.Opt.Size = jkkm.Opt.Size
	kkm.Opt.ReadTimeout = time.Duration(jkkm.Opt.ReadTimeout) * time.Millisecond

	kkm.Name = jkkm.Name
	kkm.CodePage = jkkm.CodePage
	kkm.TimeOut = jkkm.TimeOut
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(jkkm.AdminPassword))
	copy(kkm.AdminPassword[:], b[0:4])
	binary.LittleEndian.PutUint32(b, uint32(jkkm.Password))
	copy(kkm.Password[:], b[0:4])
	kkm.MaxAttemp = jkkm.MaxAttemp

	kkm.Param.Fname = jkkm.Param.Fname
	kkm.Param.Inn = jkkm.Param.Inn
	kkm.Param.KKMSerialNumber = jkkm.Param.KKMSerialNumber
	kkm.Param.RNM = jkkm.Param.RNM
	kkm.Param.LenLine = jkkm.Param.LenLine
}

func toInt(iface interface{}) int {
	var i int
	switch iface.(type) {
	case float64:
		i = int(iface.(float64))
	case float32:
		i = int(iface.(float32))
	case int64:
		i = int(iface.(int64))
	case int32:
		i = int(iface.(int32))
	case int:
		i = (iface.(int))
	case string:
		i, _ = strconv.Atoi(iface.(string))
	case uint8:
		i = int(iface.(uint8))
	case uint32:
		i = int(iface.(uint32))
	case uint64:
		i = int(iface.(uint64))
	}
	return i
}

//UnSerialize из json byte возвращает структуру kkmDrv
func UnSerialize(jdata []byte) (*KkmDrv, error) {
	var kkm = KkmDrv{}
	var dat map[string]interface{}
	if err := json.Unmarshal(jdata, &dat); err != nil {
		return nil, err
	}
	pcf, ok := dat["portconf"].(map[string]interface{}) //pcf map
	if ok {
		kkm.Opt.Name = pcf["name"].(string)
		kkm.Opt.Baud = toInt(pcf["baud"])
		kkm.Opt.Parity = serial.Parity(uint8(toInt(pcf["parity"])))
		kkm.Opt.StopBits = serial.StopBits(uint8(toInt(pcf["stopbits"])))
		kkm.Opt.Size = uint8(toInt(pcf["size"]))
		kkm.Opt.ReadTimeout = time.Duration(toInt(pcf["readtimeout"])) * time.Millisecond
	}
	kkm.Name = dat["name"].(string)
	kkm.State.Busy = false
	kkm.CodePage = dat["codepage"].(string)
	kkm.DeviceID = dat["deviceid"].(string)
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(toInt(dat["adminpassword"])))
	copy(kkm.AdminPassword[:], b[0:4])
	binary.LittleEndian.PutUint32(b, uint32(toInt(dat["password"])))
	copy(kkm.Password[:], b[0:4])
	kkm.MaxAttemp = int64(toInt(dat["maxattempt"]))
	kkm.TimeOut = int64(toInt(dat["timeout"]))
	kkm.Connected = false
	pcf, ok = dat["kkmparam"].(map[string]interface{})
	if ok {
		kkm.Param.Fname = pcf["fname"].(string)
		kkm.Param.KKMSerialNumber = pcf["kkmserialnum"].(string)
		kkm.Param.Inn = pcf["inn"].(string)
		rnm, ok1 := pcf["rnm"]
		if ok1 {
			kkm.Param.RNM = rnm.(string)
		}
	}
	return &kkm, nil
}

//ParseErrState статус ошибки ККМ
func (kkm *KkmDrv) ParseErrState(errnum byte) string {
	kkmerr := map[byte]string{
		0x00: "ошибок нет",
		0x01: "Неизвестная команда, неверный формат посылки или неизвестные параметры",
		0x02: "Неверное состояние ФН",
		0x03: "Ошибка ФН",
		0x04: "Ошибка КС, некорректные параметры в команде обращения к фп",
		0x05: "Закончен срок эксплуатации ФН",
		0x06: "Архив ФН переполнен",
		0x07: "Неверные дата и/или время",
		0x08: "Нет запрошенных данных, команда не поддерживается в данной реализации фп",
		0x09: "некорректная длина команды",
		0x0a: "формат данных не bcd",
		0x0b: "неисправна ячейка памяти фп при записи итога",
		0x10: "ФН Превышение размеров TLV данных",
		0x11: "Нет транспортного соединения",
		0x12: "Исчерпан ресурс КС (криптографического сопроцессора)",
		0x13: "текущая дата меньше даты последней записи в фп",
		0x14: "Исчерпан ресурс хранения",
		0x15: "смена уже открыта, Исчерпан ресурс ожидания передачи сообщения",
		0x16: "Продолжительность смены более 24	часов",
		0x17: "Неверная разница во времени между 2 операциями",
		0x18: "дата первой смены больше даты последней смены",
		0x19: "нет данных в фп",
		0x1a: "область перерегистраций в фп переполнена",
		0x1b: "заводской номер не введен",
		0x1c: "в заданном диапазоне есть поврежденная запись",
		0x1d: "повреждена последняя запись сменных итогов",
		0x1e: "область перерегистраций фп переполнена",
		0x1f: "отсутствует память регистров",

		0x20: "Сообщение от ОФД не может быть принято",
		0x21: "вычитаемая сумма больше содержимого денежного регистра",
		0x22: "неверная дата",
		0x23: "нет записи активизации",
		0x24: "область активизаций переполнена",
		0x25: "нет активизации с запрашиваемым номером",
		0x26: "в фп присутствует 3 или более битых записей сменных итогов",
		0x27: "признак несовпадения кс, з/н, перерегистраций или активизаций",

		0x2b: "невозможно отменить предыдущую команду",
		0x2c: "обнулѐнная касса (повторное гашение невозможно)",
		0x2d: "сумма чека по секции меньше суммы сторно",
		0x2e: "в ккт нет денег для выплаты",
		0x2f: "Таймаут обмена с ФН",
		0x30: "ФН не отвечает (ккт заблокирован, ждет ввода пароля налогового инспектора)",

		0x32: "требуется выполнение общего гашения",
		0x33: "некорректные параметры в команде",
		0x34: "нет данных",
		0x35: "некорректный параметр при данных настройках",
		0x36: "некорректные параметры в команде для данной реализации ккт",
		0x37: "команда не поддерживается в данной реализации ккт",
		0x38: "ошибка в пзу",
		0x39: "внутренняя ошибка по ккт",
		0x3a: "переполнение накопления по надбавкам в смене",
		0x3b: "переполнение накопления в смене",
		0x3c: "эклз: неверный регистрационный номер",
		0x3d: "смена не открыта – операция невозможна",
		0x3e: "переполнение накопления по секциям в смене",
		0x3f: "переполнение накопления по скидкам в смене",

		0x40: "переполнение диапазона скидок",
		0x41: "переполнение диапазона оплаты наличными",
		0x42: "переполнение диапазона оплаты типом 2",
		0x43: "переполнение диапазона оплаты типом 3",
		0x44: "переполнение диапазона оплаты типом 4",
		0x45: "cумма всех типов оплаты меньше итога чека",
		0x46: "не хватает наличности в кассе",
		0x47: "переполнение накопления по налогам в смене",
		0x48: "переполнение итога чека",
		0x49: "операция невозможна в открытом чеке данного типа",
		0x4a: "открыт чек – операция невозможна",
		0x4b: "буфер чека переполнен",
		0x4c: "переполнение накопления по обороту налогов в смене",
		0x4d: "вносимая безналичной оплатой сумма больше суммы чека",
		0x4e: "смена превысила 24 часа",
		0x4f: "неверный пароль",

		0x50: "идет печать предыдущей команды",
		0x51: "переполнение накоплений наличными в смене",
		0x52: "переполнение накоплений по типу оплаты 2 в смене",
		0x53: "переполнение накоплений по типу оплаты 3 в смене",
		0x54: "переполнение накоплений по типу оплаты 4 в смене",
		0x55: "чек закрыт – операция невозможна",
		0x56: "нет документа для повтора",
		0x57: "эклз: количество закрытых смен не совпадает с фп",
		0x58: "ожидание команды продолжения печати",
		0x59: "документ открыт другим оператором",
		0x5a: "скидка превышает накопления в чеке",
		0x5b: "переполнение диапазона надбавок",
		0x5c: "понижено напряжение 24в",
		0x5d: "таблица не определена",
		0x5e: "некорректная операция",
		0x5f: "отрицательный итог чека",

		0x60: "переполнение при умножении",
		0x61: "переполнение диапазона цены",
		0x62: "переполнение диапазона количества",
		0x63: "переполнение диапазона отдела",
		0x64: "фп отсутствует",
		0x65: "не хватает денег в секции",
		0x66: "переполнение денег в секции",
		0x67: "ошибка связи с фп",
		0x68: "не хватает денег по обороту налогов",
		0x69: "переполнение денег по обороту налогов",
		0x6a: "ошибка питания в момент ответа по i2c",
		0x6b: "нет чековой ленты",
		0x6c: "нет контрольной ленты",
		0x6d: "не хватает денег по налогу",
		0x6e: "переполнение денег по налогу",
		0x6f: "переполнение по выплате в смене",

		0x70: "переполнение фп",
		0x71: "ошибка отрезчика",
		0x72: "команда не поддерживается в данном подрежиме",
		0x73: "команда не поддерживается в данном режиме",
		0x74: "ошибка озу",
		0x75: "ошибка питания",
		0x76: "ошибка принтера: нет импульсов с тахогенератора",
		0x77: "ошибка принтера: нет сигнала с датчиков",
		0x78: "замена по",
		0x79: "замена фп",
		0x7a: "поле не редактируется",
		0x7b: "ошибка оборудования",
		0x7c: "не совпадает дата",
		0x7d: "неверный формат даты",
		0x7e: "неверное значение в поле длины",
		0x7f: "переполнение диапазона итога чека",

		0x80: "ошибка связи с фп",
		0x81: "ошибка связи с фп",
		0x82: "ошибка связи с фп",
		0x83: "ошибка связи с фп",
		0x84: "переполнение наличности",
		0x85: "переполнение по продажам в смене",
		0x86: "переполнение по покупкам в смене",
		0x87: "переполнение по возвратам продаж в смене",
		0x88: "переполнение по возвратам покупок в смене",
		0x89: "переполнение по внесению в смене",
		0x8a: "переполнение по надбавкам в чеке",
		0x8b: "переполнение по скидкам в чеке",
		0x8c: "отрицательный итог надбавки в чеке",
		0x8d: "отрицательный итог скидки в чеке",
		0x8e: "отрицательный итог скидки в чеке",
		0x8f: "касса не фискализирована",

		0x90: "поле превышает размер, установленный в настройках",
		0x91: "выход за границу поля печати при данных настройках шрифта",
		0x92: "наложение полей",
		0x93: "восстановление озу прошло успешно",
		0x94: "исчерпан лимит операций в чеке",
		0x95: "неизвестная ошибка эклз",

		0xa0: "ошибка связи с эклз",
		0xa1: "эклз отсутствует",
		0xa2: "эклз: некорректный формат или параметр команды",
		0xa3: "некорректное состояние эклз",
		0xa4: "авария эклз",
		0xa5: "авария кс в составе эклз",
		0xa6: "исчерпан временной ресурс эклз",
		0xa7: "эклз переполнена",
		0xa8: "эклз: неверные дата и время",
		0xa9: "эклз: нет запрошенных данных",
		0xaa: "переполнение эклз (отрицательный итог документа)",

		0xb0: "эклз: переполнение в параметре количество",
		0xb1: "эклз: переполнение в параметре сумма",
		0xb2: "эклз: уже активизирована",

		0xc0: "контроль даты и времени (подтвердите дату и время)",
		0xc1: "эклз: суточный отчѐт с гашением прервать нельзя",
		0xc2: "превышение напряжения в блоке питания",
		0xc3: "несовпадение итогов чека и эклз",
		0xc4: "несовпадение номеров смен",
		0xc5: "буфер подкладного документа пуст",
		0xc6: "подкладной документ отсутствует",
		0xc7: "поле не редактируется в данном режиме",
		0xc8: "отсутствуют импульсы от таходатчика",
		0xc9: "перегрев печатающей головки",
		0xca: "температура вне условий эксплуатации",
	}
	return kkmerr[errnum]
}

//ParseState вернет режим ККТ
func (kkm *KkmDrv) ParseState(state byte) (int, string) {
	//Режим ККМ – одно из состояний ККМ, в котором она может находиться.
	//Режимы ККМ описываются одним байтом: младший полубайт – номер режима,
	// старший полубайт – битовое поле, определяющее статус режима (для режимов 8, 13 и 14).
	// Номера и назначение режимов и статусов:

	num := int(state & 0b00001111)
	mode := int(state >> 4) //старший полубайт status
	if num == 8 || num == 13 || num == 14 {
		mode = num*10 + mode
	} else {
		mode = num
	}
	kkmode := map[int]string{
		0:   "Принтер в рабочем режиме.",
		1:   "Выдача данных.",
		2:   "Открытая смена, 24 часа не кончились.",
		3:   "Открытая смена, 24 часа кончились.",
		4:   "Закрытая смена.",
		5:   "Блокировка по неправильному паролю налогового инспектора.",
		6:   "Ожидание подтверждения ввода даты.",
		7:   "Разрешение изменения положения десятичной точки.",
		80:  "Открытый документ:Продажа.",
		81:  "Открытый документ:Покупка.",
		82:  "Открытый документ:Возврат продажи.",
		83:  "Открытый документ:Возврат покупки.",
		9:   "Режим разрешения технологического обнуления. В этот режим ККМ переходит по включению питания, если некорректна информация в энергонезависимом ОЗУ ККМ.",
		10:  "Тестовый прогон.",
		11:  "Печать полного фис. отчета.",
		12:  "Печать отчёта ЭКЛЗ.",
		130: "Работа с фискальным подкладным документом: Продажа (открыт).",
		131: "Работа с фискальным подкладным документом: Покупка (открыт).",
		132: "Работа с фискальным подкладным документом: Возврат продажи (открыт).",
		133: "Работа с фискальным подкладным документом: Возврат покупки (открыт).",
		140: "Печать подкладного документа: Ожидание загрузки.",
		141: "Печать подкладного документа: Загрузка и позиционирование.",
		142: "Печать подкладного документа: Позиционирование.",
		143: "Печать подкладного документа: Печать.",
		144: "Печать подкладного документа: Печать закончена.",
		145: "Печать подкладного документа: Выброс документа.",
		146: "Печать подкладного документа: Ожидание извлечения.",
		15:  "Фискальный подкладной документ сформирован.",
	}
	return num, kkmode[mode]
}

//ParseSubState подрежимы ккм
func (kkm *KkmDrv) ParseSubState(state byte) (int, string) {
	// Подрежимы ККТ
	// Подрежим ККТ – одно из состояний ККТ , в котором он может находиться.
	// Номера и назначение подрежимов:
	num := int(state)

	kkmode := map[int]string{
		0: `Бумага есть. ККТ не в фазе печати операции. может принимать от хоста команды, связанные с печатью на том документе, датчик которого сообщает о наличии бумаги.`,
		1: `Пассивное отсутствие бумаги. ККТ не в фазе печати операции – 
        не принимает от хоста команды, связанные с печатью на том 
        документе, датчик которого сообщает об отсутствии бумаги.`,
		2: `Активное отсутствие бумаги. ККТ в фазе печати операции – 
        принимает только команды, не связанные с печатью. Переход из 
        этого подрежима только в подрежим 3.`,
		3: `После активного отсутствия бумаги – ККТ ждет команду 
        продолжения печати. Кроме этого принимает команды, не 
        связанные с печатью.`,
		4: `Фаза печати операции полных фискальных отчетов – ККТ не 
        принимает от хоста команды, связанные с печатью, кроме команды
         прерывания печати.`,
		5: `Фаза печати операции – ККТ не принимает от хоста команды, 
        связанные с печатью.`,
	}
	return num, kkmode[num]
}

//ParseFlag Флаги ККТ
func (kkm *KkmDrv) ParseFlag(num uint16) (int, string) {
	flag := uint16(0b0000000000000001)
	//num := int(binary.LittleEndian.Uint16(state[0:2]))
	ret := ""
	kkmode := map[int]string{
		0:  `Рулон операционного журнала`,
		1:  `Рулон чековой ленты`,
		2:  `Верхний датчик подкладного документа`,
		3:  `Нижний датчик подкладного документа`,
		4:  `Положение десятичной точки`,
		5:  `ЭКЛЗ`,
		6:  `Оптический датчик операционного журнала`,
		7:  `Оптический датчик чековой ленты`,
		8:  `Рычаг термоголовки операционного журнала опущен`,
		9:  `Рычаг термоголовки чека опущен`,
		10: `Крышка корпуса ФР поднята`,
		11: `Денежный ящик открыт`,
		12: `Крышка корпуса ККТ контрольной ленты поднята`,
		13: `Отказ левого датчика принтера`,
		14: `ЭКЛЗ почти заполнена`,
		15: `Увеличенная точность количества`,
	}
	for i := 0; i < 16; i++ {
		switch num & flag {
		case 0:
			ret = ret + "\n" + kkmode[i] + " [нет] "
		case flag:
			ret = ret + "\n" + kkmode[i] + " [да ] "
		}
		flag = flag << 1
	}

	return int(num), ret
}

// ParseFlagFP Флаги ФП
func (kkm *KkmDrv) ParseFlagFP(flagfp byte) (int, string) {
	//Битовое поле (назначение бит):
	num := int(flagfp)
	var ret string
	switch flagfp & 0b00000001 {
	case 0:
		ret = "ФП 1 нет"
	case 1:
		ret = "ФП 1 есть"
	}
	switch flagfp & 0b00000010 {
	case 0:
		ret = ret + "\nФП 2 нет"
	case 2:
		ret = ret + "\nФП 2 есть"
	}
	switch flagfp & 0b00000100 {
	case 0:
		ret = ret + "\nЛицензия не введена"
	case 4:
		ret = ret + "\nЛицензия введена"
	}
	switch flagfp & 0b00001000 {
	case 0:
		ret = ret + "\nПереполнения ФП нет"
	case 8:
		ret = ret + "\nПереполнение ФП"
	}
	switch flagfp & 0b00010000 {
	case 0:
		ret = ret + "\nБатарея ФП >80%"
	case 16:
		ret = ret + "\nБатарея ФП <80%"
	}
	switch flagfp & 0b00100000 {
	case 0:
		ret = ret + "\nПоследняя запись ФП корректна"
	case 32:
		ret = ret + "\nПоследняя запись ФП испорчена"
	}
	switch flagfp & 0b01000000 {
	case 0:
		ret = ret + "\nСмена в ФП закрыта"
	case 64:
		ret = ret + "\nСмена в ФП открыта"
	}
	switch flagfp & 0b10000000 {
	case 0:
		ret = ret + "\n24 часа в ФП не кончились"
	case 128:
		ret = ret + "\n24 часа в ФП кончились"
	}
	/*
		0: {0: "ФП 1 нет", 1: "ФП 1 есть"},
		1: {0: "ФП 2 нет", 1: "ФП 2 есть"},
		2: {0: "Лицензия не введена", 1: "Лицензия введена"},
		3: {0: "Переполнения ФП нет", 1: "Есть переполнение ФП"},
		4: {0: "Батарея ФП >80%", 1: "Батарея ФП <80%"},
		5: {0: "Последняя запись ФП испорчена", 1: "Последняя запись ФП корректна"},
		6: {0: "Смена в ФП закрыта", 1: "Смена в ФП открыта"},
		7: {0: "24 часа в ФП не кончились", 1: "24 часа в ФП кончились"},
	*/

	return num, ret
}

// FNGetStatus Запрос статуса ФН
func (kkm *KkmDrv) FNGetStatus() (byte, error) {
	/*Код команды FF01h. Длина сообщения: 6 байт.
	  Пароль системного администратора: 4 байта
	  Ответ: FF01h Длина сообщения: 31 байт.
	  Код ошибки: 1 байт
	  Состояние фазы жизни: 1 байт
	  Бит 0 – проведена настройка ФН
	  Бит 1 – открыт фискальный режим
	  Бит 2 – закрыт фискальный режим
	  Бит 3 – закончена передача фискальных данных в ОФД
	  Текущий документ: 1 байт
	  00h – нет открытого документа
	  01h – отчет о фискализации
	  02h – отчет об открытии смены
	  04h – кассовый чек
	  08h – отчет о закрытии смены
	  10h – отчет о закрытии фискального режима
	  11h – Бланк строкой отчетности
	  12h - Отчет об изменении параметров регистрации ККТ в связи с заменой
	  ФН
	  13h – Отчет об изменении параметров регистрации ККТ
	  14h – Кассовый чек коррекции
	  15h – БСО коррекции
	  17h – Отчет о текущем состоянии расчетов
	  Данные документа: 1 байт
	  00 – нет данных документа
	  01 – получены данные документа
	  Состояние смены: 1 байт
	  00 – смена закрыта
	  01 – смена открыта
	  Флаги предупреждения: 1 байт
	  Дата и время: 5 байт
	  Номер ФН: 16 байт ASCII
	  Номер последнего ФД: 4 байта
	*/
	admpass := kkm.GetAdminPass()
	errcode, data, err := kkm.SendCommand(0xff41, admpass)
	if errcode > 0 {
		if err != nil {
			log.Printf("FNGetStatus: %v", err)
			return 1, err
		}
	}
	if len(data) > 1 {
		kkm.mu.Lock()
		kkm.FNState.FNLifeState = data[0]
		kkm.FNState.FNCurrentDocument = data[1]
		kkm.FNState.FNDocumentData = data[2]
		kkm.FNState.FNSessionState = data[3]
		kkm.FNState.FNWarningFlags = data[4]
		kkm.FNState.DateTime = binary.LittleEndian.Uint64(data[5:10])
		kkm.FNState.SerialNumber = string(data[10:26])
		kkm.FNState.DocumentNumber = binary.LittleEndian.Uint32(data[26:30])
		kkm.mu.Unlock()
		return 0, nil
	}
	return errcode, nil
}

//SetConfig установим конфиг порта, mutex-op {Name:"COM45",Baud:115200,ReadTimeout:time.Millisecond*500}
func (kkm *KkmDrv) SetConfig(c serial.Config) {
	//kkm.Opt = new(serial.Config)
	size, par, stop := c.Size, c.Parity, c.StopBits

	if size == 0 {
		size = serial.DefaultSize
	}
	if par == 0 {
		par = serial.ParityNone
	}
	if stop == 0 {
		stop = serial.Stop1
	}
	kkm.mu.Lock()
	kkm.Opt.Size = size
	//kkm.Opt.ParityNone = par
	//kkm.Opt.Stop1 = stop
	kkm.Opt.Name = c.Name
	kkm.Opt.Baud = c.Baud
	kkm.Opt.ReadTimeout = c.ReadTimeout
	kkm.mu.Unlock()
}

//OpenPort открываем порт, mutex-op
func (kkm *KkmDrv) OpenPort(c serial.Config) (err error) {
	//c := &kkm.serial.Config{Name: "COM45", Baud: 115200}
	//kkm.SetConfig(c)
	port, err := serial.OpenPort(&c)
	if err != nil {
		log.Printf("serial.Open: %v", err)
		return err
	}
	kkm.mu.Lock()
	kkm.Port = port
	kkm.mu.Unlock()
	return nil
}

//Close закрываем порт
func (kkm *KkmDrv) Close() {
	kkm.Port.Close()
}

//Write пишем в порт
func (kkm *KkmDrv) Write(buf []byte) (num int, err error) {
	num, err = kkm.Port.Write(buf)
	if err != nil {
		if err != io.EOF {
			log.Printf("port.write err: %v", err)
		}
	}
	return num, err
}

//LRC Расчет контрольной суммы
func LRC(buff []byte) byte {
	result := 0
	for _, c := range buff {
		result = result ^ int(c)
	}
	//fmt.Printf("LRC %v", result)
	return byte(result)
}

//checkState Проверяем статус ККМ, посылаем ENQ и ждем ASK или NAK
func (kkm *KkmDrv) checkState() (byte, error) {
	kkm.SendENQ()
	for x := 0; x < 3; x++ {
		a, _, err := kkm.Read(1)
		if err != nil {
			kkm.Close()
			return 0, err
		}
		switch a[0] {
		case NAK:
			return NAK, nil
		case ACK:
			return ACK, nil
		}
		kkm.SendENQ()
		//time.Sleep(kkm.Opt.ReadTimeout)
	}
	err := errors.New("Нет связи с устройством")
	kkm.Close()
	return 0, err
}

//SendENQ отправляем ENQ, пепеводит ККМ в режим ожидания команды NAK
func (kkm *KkmDrv) SendENQ() error {
	log.Println("SendENQ")
	_, err := kkm.Write([]byte{ENQ})
	//time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond) * 2))
	return err
}

//SendACK отправляем ACK
func (kkm *KkmDrv) SendACK() error {
	log.Println("SendACK")
	time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond)))
	_, err := kkm.Write([]byte{ACK})
	return err
}

//SendNAK отправляем NAK
func (kkm *KkmDrv) SendNAK() error {
	log.Println("SendNAK")
	time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond)))
	_, err := kkm.Write([]byte{NAK})
	return err
}

//SendCommand отправка команды в ККМ и возврат результата
func (kkm *KkmDrv) SendCommand(cmdint uint16, params []byte) (errcode byte, data []byte, err error) {
	//очистим параметры предидущей команды
	kkm.SetErrState(0)
	if !kkm.GetConnected() {
		_, err = kkm.Connect()
		if err != nil {
			return 1, nil, err
		}
	}
	b := make([]byte, 8)
	binary.LittleEndian.PutUint16(b, cmdint)

	cmdlen := 1
	if cmdint > 255 { //команда 0xFF__
		cmdlen = 2

	}
	content := make([]byte, cmdlen+2+len(params)) //stx+cmd+params+crc
	content[0] = STX                              //string(rune(STX))[0]
	content[1] = byte(cmdlen + len(params))       //cmd+params
	content[2] = b[0]
	if cmdlen > 1 {
		content[3] = b[1]
	}
	for i, c := range params {
		content[cmdlen+2+i] = c
	}
	crc := LRC(content[1:])
	//self.conn.write(STX+content+crc)
	//self.conn.flush()
	sending := append(content, crc)
	for i := int64(0); i < kkm.MaxAttemp; i++ {
		var num int
		var answer []byte
		log.Printf("port send %v\n", sending)
		_, err = kkm.Write(sending)
		if err != nil {
			log.Printf("SendCommand, port.Write err: %x", err)
			return 1, []byte{0}, err
		}
		answer, num, err = kkm.ReadAnswer()
		if err != nil {
			log.Printf("SendCommand, readAnswer err: %x", err)
			return 1, answer, err
			//kkm.SendENQ()
		}
		//answer[0]=cmd
		//answer[1]=код ошибки
		//Ответное сообщение содержит корректную информацию, если код ошибки (второй
		//байт в ответном сообщении) 0. Если код ошибки не 0, передается только код команды и код
		//ошибки – 2 байта
		if num > 0 && len(answer) > 1 {
			//для двухбайтных комманд
			if cmdlen > 2 && len(answer) > 2 {
				errcode = answer[2]
				if num > 3 {
					data = answer[3:num]
				} else {
					data = []byte{}
				}
			} else {
				errcode = answer[1]
				if num > 2 {
					data = answer[2:num]
				} else {
					data = []byte{}
				}
			}
			kkm.SetErrState(errcode)
			return
		}
		//if answer[0]!=NAK {

		//}
	}
	kkm.SetErrState(errcode)
	return

}

//Read читает num байт из ккм
func (kkm *KkmDrv) Read(num int) ([]byte, int, error) {
	/*data := make([]byte, num)
	n := 0
	for {
		n, err := kkm.Port.Read(data)
		if err == io.EOF { // если конец файла
			break // выходим из цикла
		}
		fmt.Print(string(data[:n]))
	}
	return data[:n], n, nil
	*/
	var buf []byte
	//if num < 2048 {
	//	buf = make([]byte, num, 2048)
	//} else {
	buf = make([]byte, num)
	//}
	n, err := kkm.Port.Read(buf)
	if err != nil {
		if err != io.EOF {
			log.Printf("Error reading from serial port: %v", err)
			return buf, 0, err
		}
	}
	log.Printf("Recv %v bytes: %v\n", n, buf)
	if n == 0 {
		return buf[:1], 0, nil
	}
	if num > n {
		num = n
	}
	//return hex.EncodeToString(buf), nil
	return buf[:num], n, nil
}

//oneRoundRead весь ответ ККМ, если int=0 чтение с ошибкой
func (kkm *KkmDrv) oneRoundRead() ([]byte, int, error) {
	a, n, err := kkm.Read(1)
	if err != nil {
		return a, 0, err
	}
	if n == 0 {
		return a, 0, nil
	}
	switch a[0] {
	case NAK:
		log.Println("exit NAK from oneRoundRead")
		return a, 0, nil
	case ACK:
		time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond) * 2))
		a, _, err = kkm.Read(1) //этот read может быть долгим
		if err != nil {
			return a, 0, err
		}
		if a[0] != STX {
			time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond) * 2))
			return a, 0, errors.New("нет связи с устройством: lost STX")
		}
		time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond) * 2))
		length, _, err := kkm.Read(1)
		if err != nil {
			return length, 0, err
		}
		time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond) * 2))
		data, _, err := kkm.Read(int(length[0]))
		if err != nil {
			return data, 0, err
		}
		time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond) * 2))
		crc, _, err := kkm.Read(1)
		if err != nil {
			return crc, 0, err
		}
		mycrc := LRC(append(length, data...))
		if crc[0] != mycrc {
			log.Printf("LRC not correct. counting=%v, receiving=%v\n", mycrc, crc)
			kkm.SendNAK()
			return nil, 0, nil
		}
		//copy(kkm.Data[:], data)
		kkm.SendACK()
		return data, int(length[0]), nil
	case STX:
		time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond) * 2))
		length, _, err := kkm.Read(1)
		if err != nil {
			return length, 0, err
		}
		time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond) * 2))
		data, _, err := kkm.Read(int(length[0]))
		if err != nil {
			return data, 0, err
		}
		time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond) * 2))
		crc, _, err := kkm.Read(1)
		if err != nil {
			return crc, 0, err
		}
		mycrc := LRC(append(length, data...))
		if crc[0] != mycrc {
			log.Printf("LRC not correct. counting=%v, receiving=%v\n", mycrc, crc)
			kkm.SendNAK()
			return nil, 0, nil
		}
		//copy(kkm.Data[:], data)
		kkm.SendACK()
		return data, int(length[0]), nil
	default:
		kkm.Port.Flush()
		return nil, 0, nil
	}
}

//ClearAnswer Сбрасывает ответ если он болтается в ККМ
func (kkm *KkmDrv) ClearAnswer() (int, error) {
	log.Println("ClearAnswer")
	//var buf []byte
	//kkm.Port.Flush()
	for i := (int64)(0); i < 2; i++ {
		a, _, err := kkm.Read(1)
		if err != nil {
			return 0, err
		}
		switch a[0] {
		case NAK:
			return 1, nil
		case STX, ACK:
			_, _, err = kkm.ReadAnswer()
			kkm.SendACK()
		default:
			kkm.SendNAK()
			time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond)))
			kkm.SendENQ()
			_, _, err = kkm.ReadAnswer()
			kkm.SendACK()
		}
	}
	//мы тут, значит ккм не успевает прочитать наш аск
	//надо уменьшить таймаут и постепенно его увеличивать, и да надо отправить коианду прерывания выдачи данных 03
	kkm.TimeOut = 1
	kkm.SendCommand(0x03, kkm.AdminPassword[:])
	kkm.Port.Flush()

	for i := (int64)(0); i < kkm.MaxAttemp; i++ {
		kkm.TimeOut = kkm.TimeOut + 5
		kkm.SendENQ()
		time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond) * 2))
		a, _, err := kkm.Read(1)
		if err != nil {
			return 0, err
		}
		switch a[0] {
		case NAK:
			return 1, nil
		case STX, ACK:
			_, _, err = kkm.ReadAnswer()
			kkm.SendACK()
		default:
			kkm.SendNAK()
			time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond) * 2))
			kkm.SendENQ()
			_, _, err = kkm.ReadAnswer()
			kkm.SendACK()
		}
	}
	return 0, nil
}

//ReadAnswer """Считать ответ ККМ"""
func (kkm *KkmDrv) ReadAnswer() ([]byte, int, error) {
	var err error
	var buf []byte
	n := int64(0)
	ret := 0
	//var err error
	for ; n < kkm.MaxAttemp; n++ {
		time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond)))
		buf, ret, err = kkm.oneRoundRead()
		if err != nil {
			return nil, 0, err
		}
		if ret > 0 {
			return buf, ret, nil
		}
		if buf != nil && buf[0] == NAK {
			// repit command
			return buf, 0, nil
		}
		kkm.SendENQ() //приняли что то не понятное, запросим повтор ответа
	}
	return nil, 0, errors.New("Не получен правильный ответ в течении " + strconv.FormatInt(kkm.MaxAttemp, 10) + " попыток") //read answe error
}

//Connect подключает ККМ
func (kkm *KkmDrv) Connect() (int, error) {

	log.Println("Connecting...")

	if kkm.GetConnected() {
		//todo: need close?
		kkm.SetConnected(false)
		kkm.Close()
	}
	kkm.mu.RLock()
	options := kkm.Opt
	kkm.mu.RUnlock()
	err := kkm.OpenPort(options)
	if err != nil {
		log.Printf("serial.Open: %v", err)
		kkm.SetConnected(false)
		return 0, err
		//panic("dont open port")
	}
	log.Println("port is opening")
	kkm.Port.Flush()
	for n := (int64)(0); n < kkm.MaxAttemp; n++ {
		ret, err := kkm.checkState() //=kkm.SendENQ() and read
		if err != nil {
			return 0, err
		}
		switch ret {
		case NAK:
			kkm.SetConnected(true)
			log.Println("Wait command state")
			return 1, nil
		case ACK:
			//wait stx
			log.Println("KKM status ASK")
			kkm.ClearAnswer()
			kkm.SetConnected(true)
			return 1, nil
		default:
			log.Printf("Check connection@ KKM in silens %v", ret)
			kkm.ClearAnswer()
		}
	}
	kkm.Close()
	return 0, errors.New("Check connection@ KKM in bad state")
}

func main() {
}
