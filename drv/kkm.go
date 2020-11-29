package drv

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log"
	"strconv"
	"time"

	"github.com/tarm/serial"
	// bolt "go.etcd.io/bbolt"
)

//ENQ команда ККМ для перевода в режим ожидания команды
const ENQ = 0x05

//STX команда от ККМ указывает что следом идут данные
const STX = 0x02

//ACK команда ККМ означает все ОК, принял
const ACK = 0x06

//NAK команда ККМ означает что была ошибка приема
const NAK = 0x15

//MaxAttemp максимальное число попыток приема передачи одной команды
const MaxAttemp = 4

//KkmDrv структура драйвера
type KkmDrv struct {
	DeviceID      string
	Port          *serial.Port
	Opt           serial.Config
	TimeOut       int64
	Connected     bool
	Password      [4]byte
	AdminPassword [4]byte
	MaxAttemp     int64
	CodePage      string
	Data          [1024]byte
	Busy          bool
	Param         KkmParam
	State         KkmState
}

//KkmParam параметры модели, серийный номер, ИНН и пр
type KkmParam struct {
	KKMNumber string `json:"kkmserialnum"`
	Inn       string `json:"inn"`
	Fname     string `json:"fname"`
}

//PortConf копия конфигурации порта для сериализации
type PortConf struct {
	Name        string        `json:"name"`
	Baud        int           `json:"baud"`
	ReadTimeout time.Duration `json:"readtimeout"` // Total timeout
	// Size is the number of data bits. If 0, DefaultSize is used.
	Size byte `json:"size"`
	// Parity is the bit to use and defaults to ParityNone (no parity bit).
	Parity byte `json:"parity"`
	// Number of stop bits to use. Default is 1 (1 stop bit).
	StopBits byte `json:"stopbits"`
}

//KkmDrvSer копия конфигурации ккм для сериализации
type KkmDrvSer struct {
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
	NumState  int
	NameState string
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
func digit2str(d uint32) []byte {
	//var b bytes.Buffer
	//fmt.Fprintf(&b,"%v",d)
	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, d)
	return bs
}

//Serialize преобразует данные в json byte
func (kkm *KkmDrv) Serialize() ([]byte, error) {
	return json.Marshal(kkm.GetStruct())
}

//GetStruct преобразует данные в json byte
func (kkm *KkmDrv) GetStruct() KkmDrvSer {
	var pconf = PortConf{}
	pconf.Name = kkm.Opt.Name
	pconf.Baud = kkm.Opt.Baud
	pconf.Parity = byte(kkm.Opt.Parity)
	pconf.StopBits = byte(kkm.Opt.StopBits)
	pconf.Size = byte(kkm.Opt.Size)
	pconf.ReadTimeout = kkm.Opt.ReadTimeout
	var param = KkmParam{}
	param.Fname = kkm.Param.Fname
	param.Inn = kkm.Param.Inn
	param.KKMNumber = kkm.Param.KKMNumber
	var sr = KkmDrvSer{}
	res := binary.LittleEndian.Uint32(kkm.AdminPassword[:])
	sr.AdminPassword = int64(res)
	sr.CodePage = kkm.CodePage
	sr.DeviceID = kkm.DeviceID
	sr.MaxAttemp = kkm.MaxAttemp
	sr.Password = int64(binary.LittleEndian.Uint32(kkm.Password[:]))
	sr.Opt = pconf
	sr.TimeOut = kkm.TimeOut
	sr.Param = param
	return sr
}

//SetDataFromStruct заполняет данные из структуры, которая десериализуется из json byte
func (kkm *KkmDrv) SetDataFromStruct(jkkm *KkmDrvSer) {

	kkm.Opt.Name = jkkm.Opt.Name
	kkm.Opt.Baud = jkkm.Opt.Baud
	kkm.Opt.Parity = serial.Parity(jkkm.Opt.Parity)
	kkm.Opt.StopBits = serial.StopBits(jkkm.Opt.StopBits)
	kkm.Opt.Size = jkkm.Opt.Size
	kkm.Opt.ReadTimeout = time.Duration(jkkm.Opt.ReadTimeout) * time.Millisecond

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
	kkm.Param.KKMNumber = jkkm.Param.KKMNumber

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
	kkm.Busy = false
	kkm.CodePage = dat["codepage"].(string)
	kkm.DeviceID = dat["deviceid"].(string)
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(toInt(dat["adminpassword"])))
	copy(kkm.AdminPassword[:], b[0:4])
	binary.LittleEndian.PutUint32(b, uint32(toInt(dat["password"])))
	copy(kkm.Password[:], b[0:4])
	kkm.MaxAttemp = int64(toInt(dat["maxattempt"]))
	kkm.Connected = false
	pcf, ok = dat["kkmparam"].(map[string]interface{})
	if ok {
		kkm.Param.Fname = pcf["fname"].(string)
		kkm.Param.KKMNumber = pcf["kkmserialnum"].(string)
		kkm.Param.Inn = pcf["inn"].(string)
	}
	return &kkm, nil
}

//ErrState статус ошибки ККМ
func (kkm *KkmDrv) ErrState(errnum byte) string {
	kkmerr := map[byte]string{
		0x00: "ошибок нет",
		0x01: "неисправен накопитель фп 1, фп 2 или часы",
		0x02: "отсутствует фп 1",
		0x03: "отсутствует фп 2",
		0x04: "некорректные параметры в команде обращения к фп",
		0x05: "нет запрошенных данных",
		0x06: "фп в режиме вывода данных",
		0x07: "некорректные параметры в команде для данной реализации фп",
		0x08: "команда не поддерживается в данной реализации фп",
		0x09: "некорректная длина команды",
		0x0a: "формат данных не bcd",
		0x0b: "неисправна ячейка памяти фп при записи итога",

		0x11: "не введена лицензия",
		0x12: "заводской номер уже введен",
		0x13: "текущая дата меньше даты последней записи в фп",
		0x14: "область сменных итогов фп переполнена",
		0x15: "смена уже открыта",
		0x16: "смена не открыта",
		0x17: "номер первой смены больше номера последней смены",
		0x18: "дата первой смены больше даты последней смены",
		0x19: "нет данных в фп",
		0x1a: "область перерегистраций в фп переполнена",
		0x1b: "заводской номер не введен",
		0x1c: "в заданном диапазоне есть поврежденная запись",
		0x1d: "повреждена последняя запись сменных итогов",
		0x1e: "область перерегистраций фп переполнена",
		0x1f: "отсутствует память регистров",

		0x20: "переполнение денежного регистра при добавлении",
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

		0x30: "ккт заблокирован, ждет ввода пароля налогового инспектора",

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
func (kkm *KkmDrv) ParseState(state []byte) (int, string) {
	//Режим ККМ – одно из состояний ККМ, в котором она может находиться.
	//Режимы ККМ описываются одним байтом: младший полубайт – номер режима,
	// старший полубайт – битовое поле, определяющее статус режима (для режимов 8, 13 и 14).
	// Номера и назначение режимов и статусов:

	num := int(state[0] & 0b00001111)
	mode := int(state[0] >> 4) //старший полубайт status
	if num == 8 || num == 13 || num == 14 {
		mode = num*10 + mode
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
		140: "Печать подкладного документа. Ожидание загрузки.",
		141: "Загрузка и позиционирование.",
		142: "Позиционирование.",
		143: "Печать.",
		144: "Печать закончена.",
		145: "Выброс документа.",
		146: "Ожидание извлечения.",
		15:  "Фискальный подкладной документ сформирован.",
	}
	kkm.State = KkmState{num, kkmode[mode]}
	return num, kkmode[mode]
}

//SetConfig Name: "COM45", Baud: 115200, ReadTimeout: time.Millisecond * 500
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
	kkm.Opt.Size = size
	//kkm.Opt.ParityNone = par
	//kkm.Opt.Stop1 = stop
	kkm.Opt.Name = c.Name
	kkm.Opt.Baud = c.Baud
	kkm.Opt.ReadTimeout = c.ReadTimeout
}

//OpenPort открываем порт
func (kkm *KkmDrv) OpenPort(c serial.Config) (err error) {
	//c := &kkm.serial.Config{Name: "COM45", Baud: 115200}
	kkm.SetConfig(c)
	port, err := serial.OpenPort(&kkm.Opt)
	if err != nil {
		log.Printf("serial.Open: %v", err)
		return err
	}
	kkm.Port = port
	return nil
}

//Close закрываем порт
func (kkm *KkmDrv) Close() {
	kkm.Port.Close()
}

//Write закрываем порт
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
	for x := 0; x < 2; x++ {
		a, _, err := kkm.Read(1)
		if err != nil {
			return 0, err
		}
		switch a[0] {
		case NAK:
			return NAK, nil
		case ACK:
			return ACK, nil
		}
		time.Sleep(kkm.Opt.ReadTimeout * time.Millisecond)
	}
	err := errors.New("Нет связи с устройством")
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
	time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond) * 2))
	_, err := kkm.Write([]byte{ACK})
	return err
}

//SendNAK отправляем NAK
func (kkm *KkmDrv) SendNAK() error {
	log.Println("SendNAK")
	time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond) * 2))
	_, err := kkm.Write([]byte{NAK})
	return err
}

//SendCommand отправка команды в ККМ и возврат результата
func (kkm *KkmDrv) SendCommand(cmdint uint16, params []byte) (errcode byte, data []byte, err error) {
	/*err := kkm.Port.Flush()
	if err != nil {
		if err != io.EOF {
			log.Fatalf("port.Flush err: %v", err)
			return 0, err
		}
	}*/
	if !kkm.Connected {
		_, err = kkm.Connect()
		if err != nil {
			return 0, nil, err
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

	log.Printf("port send %v\n", sending)
	_, err = kkm.Write(sending)
	if err != nil {
		log.Printf("SendCommand, port.Write err: %v", err)
		return 0, []byte{0}, err
	}
	answer, num, err := kkm.ReadAnswer()
	if err != nil {
		log.Printf("SendCommand, readAnswer err: %v", err)
		return 0, answer, err
		//kkm.SendENQ()
	}
	//answer[0]=cmd
	//answer[1]=код ошибки

	if cmdlen > 2 {
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
	buf := make([]byte, num)
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
	kkm.Port.Flush()
	kkm.SendENQ()
	a, _, err := kkm.Read(1)
	if err != nil {
		return 0, err
	}
	switch a[0] {
	case NAK:
		return 1, nil
	}
	/*
		for i:=0; i< MaxAttemp;i++{
			kkm.SendENQ()
			time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond) * 2))
			a, _, err := kkm.Read(1)
			if err != nil {
				return 0, err
			}
			if a[0]== NAK{
			return 1, nil
			}
		}
	*/

	buf := make([]byte, 2048)
	for i := 0; i < MaxAttemp; i++ {
		kkm.SendENQ()
		time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond) * 2))
		n, err := kkm.Port.Read(buf)
		if err != nil {
			if err != io.EOF {
				return 0, err
			}
			//EOF
			if buf[0] == NAK {
				return 1, nil
			}
		}
		if n > 0 {
			if buf[0] == NAK {
				return 1, nil
			}
		}
	}

	return 0, nil
}

//ReadAnswer """Считать ответ ККМ"""
func (kkm *KkmDrv) ReadAnswer() ([]byte, int, error) {
	var err error
	var buf []byte
	n := 0
	ret := 0
	//var err error
	for ; n < MaxAttemp; n++ {
		time.Sleep(time.Duration(kkm.TimeOut * int64(time.Millisecond) * 2))
		buf, ret, err = kkm.oneRoundRead()
		if err != nil {
			return nil, 0, err
		}
		if ret > 0 {
			return buf, ret, nil
		}
		kkm.SendENQ() //приняли что то не понятное, запросим повтор ответа
	}
	return nil, 0, errors.New("Не получен правильный ответ в течении " + string(MaxAttemp) + " попыток") //read answe error
}

//Connect подключает ККМ
func (kkm *KkmDrv) Connect() (int, error) {
	log.Println("Connecting...")
	options := kkm.Opt
	if kkm.Connected {
		kkm.Close()
		kkm.Connected = false
	}
	err := kkm.OpenPort(options)
	if err != nil {
		log.Printf("serial.Open: %v", err)
		return 0, err
		//panic("dont open port")
	}
	log.Println("port is opening")
	for n := 0; n < MaxAttemp; n++ {
		ret, err := kkm.checkState()
		if err != nil {
			return 0, err
		}
		switch ret {
		case NAK:
			kkm.Connected = true
			log.Println("Wait command state")
			return 1, nil
		case ACK:
			log.Println("KKM status ASK")
			kkm.ClearAnswer()
			kkm.Connected = true
			return 1, nil
		default:
			log.Printf("Check connection@ KKM in silens %v", ret)
			kkm.ClearAnswer()
		}
	}
	return 0, errors.New("Check connection@ KKM in bad state")
}

func main() {
}
