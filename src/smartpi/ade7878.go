/*
    Copyright (C) Jens Ramhorst
	  This file is part of SmartPi.
    SmartPi is free software: you can redistribute it and/or modify
    it under the terms of the GNU General Public License as published by
    the Free Software Foundation, either version 3 of the License, or
    (at your option) any later version.
    SmartPi is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU General Public License for more details.
    You should have received a copy of the GNU General Public License
    along with SmartPi.  If not, see <http://www.gnu.org/licenses/>.
    Diese Datei ist Teil von SmartPi.
    SmartPi ist Freie Software: Sie können es unter den Bedingungen
    der GNU General Public License, wie von der Free Software Foundation,
    Version 3 der Lizenz oder (nach Ihrer Wahl) jeder späteren
    veröffentlichten Version, weiterverbreiten und/oder modifizieren.
    SmartPi wird in der Hoffnung, dass es nützlich sein wird, aber
    OHNE JEDE GEWÄHRLEISTUNG, bereitgestellt; sogar ohne die implizite
    Gewährleistung der MARKTFÄHIGKEIT oder EIGNUNG FÜR EINEN BESTIMMTEN ZWECK.
    Siehe die GNU General Public License für weitere Details.
    Sie sollten eine Kopie der GNU General Public License zusammen mit diesem
    Programm erhalten haben. Wenn nicht, siehe <http://www.gnu.org/licenses/>.
*/

package smartpi

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/nathan-osman/go-rpigpio"
	"golang.org/x/exp/io/i2c"

	log "github.com/Sirupsen/logrus"
)

const (
	ADE7878_ADDR int     = 0x38
	SAMPLES      int     = 100
	ade7878Clock float64 = 256000
	halfCircle   float64 = math.Pi / 180.0
)

type CTFactors struct {
	CurrentResistor, CurrentClampFactor, OffsetCurrent, OffsetVoltage, PowerCorrectionFactor float64
}

var (
	CTTypes = map[string]CTFactors{
		"YHDC_SCT013": CTFactors{
			CurrentResistor:       7.07107,
			CurrentClampFactor:    0.05,
			OffsetCurrent:         1.049084906,
			OffsetVoltage:         1.0,
			PowerCorrectionFactor: 0.019413,
		},
		"X/1A": CTFactors{
			CurrentResistor:       0.33,
			CurrentClampFactor:    1.0,
			OffsetCurrent:         1.010725941,
			OffsetVoltage:         1.0,
			PowerCorrectionFactor: 0.043861,
		},
	}
)

var (
	rms_factor_current float64
)

// Fetch a number of bytes from the device and convert it to an int.
func DeviceFetchInt(d *i2c.Device, l int, cmd []byte) int64 {
	startTime := time.Now()
	err := d.Write(cmd)
	if err != nil {
		panic(err)
	}
	data := make([]byte, l)
	err = d.Read(data)
	if err != nil {
		panic(err)
	}
	var result int64
	switch l {
	case 8:
		result = int64(binary.BigEndian.Uint64(data))
	case 4:
		result = int64(int32(binary.BigEndian.Uint32(data)))
	case 2:
		result = int64(int16(binary.BigEndian.Uint16(data)))
	default:
		panic(fmt.Errorf("Invalid byte length for int conversion %d", l))
	}
	log.Debugf("DeviceFetchInt: %s cmd: %x data: %x result: %d", time.Since(startTime), cmd, data, result)
	return result
}

func resetADE7878() {
	println("RESET")
	p, err := rpi.OpenPin(4, rpi.OUT)
	if err != nil {
		panic(err)
	}
	defer p.Close()
	p.Write(rpi.LOW)
	time.Sleep(time.Second)
	p.Write(rpi.HIGH)
}

func initPiForADE7878() {
	/*
	   p, err := rpi.OpenPin(4, rpi.OUT)
	   if err != nil {
	       panic(err)
	   }
	   defer p.Close()
	   p.Write(rpi.HIGH)*/
}

func WriteRegister(d *i2c.Device, register string, data ...byte) (err error) {
	return d.Write(append(ADE7878REG[register], data...))
}

func InitADE7878(c *Config) (*i2c.Device, error) {

	d, err := i2c.Open(&i2c.Devfs{Dev: c.I2CDevice}, ADE7878_ADDR)
	if err != nil {
		panic(err)
	}

	// 0xEC01 (CONFIG2-REGISTER)
	// 00000010 --> I2C-Lock
	//err = d.Write(append(ADE7878REG["CONFIG2"], 0x02))
	err = WriteRegister(d, "CONFIG2", 0x02)
	if err != nil {
		panic(err)
	}

	// 0xE1
	err = d.Write([]byte{0xEC})
	if err != nil {
		panic(err)
	}

	// Read i2cLock
	i2cLock := make([]byte, 1)
	err = d.Read(i2cLock)
	if err != nil {
		panic(err)
	}

	// 0xE7FE writeprotection
	err = d.Write([]byte{0xE7, 0xFE, 0xAD})
	if err != nil {
		panic(err)
	}

	// 0xE7E3 writeprotection OFF
	err = d.Write([]byte{0xE7, 0xE3, 0x00})
	if err != nil {
		panic(err)
	}

	// // 0x43B6 (HPFDIS-REGISTER)
	// err = d.Write(append(ADE7878REG["HPFDIS"], 0x00, 0x00, 0x00, 0x00})
	// if err != nil {
	//     panic(err)
	// }

	// Set the right power frequency to the COMPMODE-REGISTER.
	// 0xE60E (COMPMODE-REGISTER)
	if c.PowerFrequency == 60 {
		// 0x41FF 60Hz
		err = WriteRegister(d, "COMPMODE", 0x41, 0xFF)
	} else {
		// 0x01FF 50Hz
		err = WriteRegister(d, "COMPMODE", 0x01, 0xFF)
	}
	if err != nil {
		panic(err)
	}

	// 0x43B5 (DICOEFF-REGISTER)
	err = WriteRegister(d, "DICOEFF", 0xFF, 0x80, 0x00)
	if err != nil {
		panic(err)
	}

	//0x43AB (WTHR1-REGISTER)
	err = WriteRegister(d, "WTHR1", 0x00, 0x00, 0x00, 0x17)
	if err != nil {
		panic(err)
	}

	//0x43AC (WTHR0-REGISTER)
	err = WriteRegister(d, "WTHR0", 0x00, 0x85, 0x60, 0x16)
	if err != nil {
		panic(err)
	}

	// // 0x43AD (VARTHR1-REGISTER)
	// err = d.Write(append(ADE7878REG["VARTHR1"], 0x17, 0x85, 0x60, 0x16))
	// if err != nil {
	//     panic(err)
	// }
	//
	// // 0x43AE (VARTHR0-REGISTER)
	// err = d.Write(append(ADE7878REG["VARTHR0"], 0x17, 0x85, 0x60, 0x16))
	// if err != nil {
	//     panic(err)
	// }
	//
	// // 0x43A9 (VATHR1-REGISTER)
	// err = d.Write(append(ADE7878REG["VATHR1"], 0x17, 0x85, 0x60, 0x16))
	// if err != nil {
	//     panic(err)
	// }
	//
	// // 0x43AA (VATHR0-REGISTER)
	// err = d.Write(append(ADE7878REG["VATHR0"], 0x17, 0x85, 0x60, 0x16))
	// if err != nil {
	//     panic(err)
	// }

	// 0x43B3 (VLEVEL-REGISTER)
	err = WriteRegister(d, "VLEVEL", 0x00, 0x0C, 0xEC, 0x85)
	if err != nil {
		panic(err)
	}

	time.Sleep(875 * time.Millisecond)

	// // 0x4381 (AVGAIN-REGISTER)
	// outcome := DeviceFetchInt(d, 4, ADE7878REG["AVGAIN"])
	// fmt.Printf("AVGAIN-REGISTER VORHER%g   %x %x %x %x \n", outcome, data[0], data[1], data[2], data[3])

	// 0x4381 (AVGAIN-REGISTER)
	err = WriteRegister(d, "AVGAIN", 0xFF, 0xFC, 0x1C, 0xC2)
	if err != nil {
		panic(err)
	}

	// 0x4383 (BVGAIN-REGISTER)
	// err = WriteRegister(d, "BVGAIN", 0xFF, 0xFB, 0xCA, 0x60)
	err = WriteRegister(d, "BVGAIN", 0xFF, 0xFC, 0x1C, 0xC2)
	if err != nil {
		panic(err)
	}

	// 0x4385 (CVGAIN-REGISTER)
	//err = WriteRegister(d, "CVGAIN", 0xFF, 0xFC, 0x12, 0xDE)
	err = WriteRegister(d, "CVGAIN", 0xFF, 0xFC, 0x1C, 0xC2)
	if err != nil {
		panic(err)
	}

	// err = WriteRegister(d, "AIRMSOS", 0x11, 0x47, 0xE9)
	// if err != nil {
	// 	panic(err)
	// }

	// Line cycle mode
	// 0xE702 LCYCMODE
	err = WriteRegister(d, "LCYCMODE", 0x0F)
	if err != nil {
		panic(err)
	}

	// Line cycle mode count
	// 0xE60C LINECYC
	err = WriteRegister(d, "LINECYC", 0xC8)
	if err != nil {
		panic(err)
	}

	// 0xE7FE writeprotection
	err = d.Write([]byte{0xE7, 0xFE, 0xAD})
	if err != nil {
		panic(err)
	}

	// 0xE7E3 writeprotection
	err = d.Write([]byte{0xE7, 0xE3, 0x80})
	if err != nil {
		panic(err)
	}

	// 0xE228 (RUN-Register)
	err = WriteRegister(d, "RUN", 0x00, 0x01)
	if err != nil {
		panic(err)
	}

	return d, nil
}

func ReadCurrent(d *i2c.Device, c *Config, phase string) (current float64) {
	command := make([]byte, 2)
	switch phase {
	case "A":
		command = ADE7878REG["AIRMS"] // 0x43C0 (AIRMS; Current rms an A)
	case "B":
		command = ADE7878REG["BIRMS"] // 0x43C2 (AIRMS; Current rms an B)
	case "C":
		command = ADE7878REG["CIRMS"] // 0x43C4 (AIRMS; Current rms an C)
	case "N":
		command = ADE7878REG["NIRMS"] // 0x43C6 (AIRMS; Current rms an N)
	default:
		panic(fmt.Errorf("Invalid phase %q", phase))
	}

	var rmsFactor float64
	switch c.PowerFrequency {
	case 60:
		rmsFactor = 3493258.0 // 60Hz
	case 50:
		rmsFactor = 4191910.0 // 50Hz
	default:
		panic(fmt.Errorf("Invalid frequency %g", c.PowerFrequency))
	}

	if c.MeasureCurrent[phase] {
		outcome := float64(DeviceFetchInt(d, 4, command))
		cr := CTTypes[c.CTType[phase]].CurrentResistor

		var ccf float64
		if c.CTType[phase] == "YHDC_SCT013" {
			ccf = CTTypes[c.CTType[phase]].CurrentClampFactor
		} else {
			ccf = 1.0 / (float64(c.CTTypePrimaryCurrent[phase]) / 100.0)
		}

		oc := CTTypes[c.CTType[phase]].OffsetCurrent
		outcome = outcome - 7300
		current = ((((outcome * 0.3535) / rmsFactor) / cr) / ccf) * 100.0 * oc
	} else {
		current = 0.0
	}
	return current
}

func ReadVoltage(d *i2c.Device, c *Config, phase string) (voltage float64, measureVoltage bool) {
	command := make([]byte, 2)
	switch phase {
	case "A":
		command = []byte{0x43, 0xC1} // 0x43C1 (AVRMS; Voltage RMS phase A)
	case "B":
		command = []byte{0x43, 0xC3} // 0x43C3 (BVRMS; Voltage RMS phase B)
	case "C":
		command = []byte{0x43, 0xC5} // 0x43C5 (BVRMS; Voltage RMS phase C)
	default:
		panic(fmt.Errorf("Invalid phase %q", phase))
	}

	voltage = float64(DeviceFetchInt(d, 4, command)) / 1e+4

	// Ignore voltage reading if disalbed or less than 10 volts.
	measureVoltage = true
	// if !c.MeasureVoltage[phase] || voltage < 10 {
	// 	voltage = c.Voltage[phase]
	// 	measureVoltage = false
	// }

	if !c.MeasureVoltage[phase] { // || voltage < 10 {
		voltage = c.Voltage[phase]
		measureVoltage = false
	}

	return voltage, measureVoltage
}

func ReadActiveWatts(d *i2c.Device, c *Config, phase string) (watts float64) {
	command := make([]byte, 2)
	switch phase {
	case "A":
		command = []byte{0xE5, 0x13} // 0xE513 (AWATT total active power phase A)
	case "B":
		command = []byte{0xE5, 0x14} // 0xE514 (BWATT total active power phase B)
	case "C":
		command = []byte{0xE5, 0x15} // 0xE515 (CWATT total active power phase C)
	default:
		panic(fmt.Errorf("Invalid phase %q", phase))
	}

	var pcf float64
	if c.CTType[phase] == "YHDC_SCT013" {
		pcf = 1.0
	} else {
		pcf = 200.0 / (float64(c.CTTypePrimaryCurrent[phase]))
	}

	outcome := float64(DeviceFetchInt(d, 4, command))
	if c.MeasureCurrent[phase] {
		watts = outcome * CTTypes[c.CTType[phase]].PowerCorrectionFactor / pcf
	} else {
		watts = 0.0
	}
	if c.CurrentDirection[phase] {
		watts *= -1
	}

	return watts
}

func ReadActiveEnergy(d *i2c.Device, c *Config, phase string) (energy float64) {
	command := make([]byte, 2)
	switch phase {
	case "A":
		command = []byte{0xE4, 0x00} // 0xE4000 (AWATTHR total active energy phase A)
	case "B":
		command = []byte{0xE4, 0x00} // 0xE4001 (BWATTHR total active energy phase B)
	case "C":
		command = []byte{0xE4, 0x00} // 0xE4002 (CWATTHR total active energy phase C)
	default:
		panic(fmt.Errorf("Invalid phase %q", phase))
	}

	var pcf float64
	if c.CTType[phase] == "YHDC_SCT013" {
		pcf = 1.0
	} else {
		pcf = 200.0 / (float64(c.CTTypePrimaryCurrent[phase]))
	}

	outcome := float64(DeviceFetchInt(d, 4, command))

	energy = outcome / pcf

	// if c.CurrentDirection[phase] {
	// 	watts *= -1
	// }

	return energy
}

func ReadAngle(d *i2c.Device, c *Config, phase string) (angle float64) {
	command := make([]byte, 2)
	switch phase {
	case "A":
		command = []byte{0xE6, 0x01} // 0xE601 (ANGLE0 cosphi an A)
	case "B":
		command = []byte{0xE6, 0x02} // 0xE602 (ANGLE1 cosphi an B)
	case "C":
		command = []byte{0xE6, 0x03} // 0xE603 (ANGLE2 cosphi an C)
	default:
		panic(fmt.Errorf("Invalid phase %q", phase))
	}

	if c.MeasureVoltage[phase] {
		outcome := float64(DeviceFetchInt(d, 2, command))
		angle = math.Cos(outcome * 360 * c.PowerFrequency / ade7878Clock * halfCircle)
		if c.CurrentDirection[phase] {
			angle *= -1
		}
	} else {
		angle = 1.0
	}

	return angle
}

func ReadFrequency(d *i2c.Device, c *Config, phase string) (frequency float64) {
	command := make([]byte, 2)
	switch phase {
	case "A":
		command = []byte{0xE7, 0x00, 0x1C} // 0xE7001C MMODE-Register measure frequency at VA
	case "B":
		command = []byte{0xE7, 0x00, 0x1D} // 0xE7001D MMODE-Register measure frequency at VB
	case "C":
		command = []byte{0xE7, 0x00, 0x1E} // 0xE7001E MMODE-Register measure frequency at VC
	default:
		panic(fmt.Errorf("Invalid phase %q", phase))
	}

	err := d.Write(command) // MMODE-Register measure frequency
	if err != nil {
		panic(err)
	}
	time.Sleep(50 * time.Millisecond)
	// 0xE607 (PERIOD)
	outcome := float64(DeviceFetchInt(d, 2, []byte{0xE6, 0x07}))
	frequency = ade7878Clock / (outcome + 1)

	return frequency
}

func ReadApparentPower(d *i2c.Device, c *Config, phase string) float64 {
	command := make([]byte, 2)
	switch phase {
	case "A":
		command = []byte{0xE5, 0x19} // 0xE519 (AVA total apparent power phase A)
	case "B":
		command = []byte{0xE5, 0x1A} // 0xE51A (BVA total apparent power phase B)
	case "C":
		command = []byte{0xE5, 0x1B} // 0xE51B (CVA total apparent power phase C)
	default:
		panic(fmt.Errorf("Invalid phase %q", phase))
	}

	var pcf float64
	if c.CTType[phase] == "YHDC_SCT013" {
		pcf = 1.0
	} else {
		pcf = 200.0 / (float64(c.CTTypePrimaryCurrent[phase]))
	}

	if c.MeasureCurrent[phase] {
		outcome := float64(DeviceFetchInt(d, 4, command))
		return outcome * CTTypes[c.CTType[phase]].PowerCorrectionFactor / pcf
	} else {
		return 0.0
	}
}

func ReadReactivePower(d *i2c.Device, c *Config, phase string) float64 {
	command := make([]byte, 2)
	switch phase {
	case "A":
		command = []byte{0xE5, 0x16} // 0xE516 (AVAR total reactive power phase A)
	case "B":
		command = []byte{0xE5, 0x17} // 0xE517 (AVAR total reactive power phase B)
	case "C":
		command = []byte{0xE5, 0x18} // 0xE518 (AVAR total reactive power phase C)
	default:
		panic(fmt.Errorf("Invalid phase %q", phase))
	}

	var pcf float64
	if c.CTType[phase] == "YHDC_SCT013" {
		pcf = 1.0
	} else {
		pcf = 200.0 / (float64(c.CTTypePrimaryCurrent[phase]))
	}

	if c.MeasureCurrent[phase] {

		outcome := float64(DeviceFetchInt(d, 4, command))
		if c.CurrentDirection[phase] {
			return outcome * -1 / pcf
		} else {
			return outcome / pcf
		}
	} else {
		return 0.0
	}
}

func CalculatePowerFactor(c *Config, phase string, watts float64, voltAmps float64, voltAmpsReactive float64) float64 {
	powerFactor := watts / CTTypes[c.CTType[phase]].PowerCorrectionFactor / voltAmps
	if c.MeasureCurrent[phase] {
		if math.Signbit(voltAmpsReactive) {
			return powerFactor
		} else {
			return powerFactor * -1
		}
	} else {
		return 0.0
	}
}

func ReadoutValues(d *i2c.Device, c *Config) [28]float64 {
	var values [28]float64
	startTime := time.Now()

	// Measure Currents
	values[0] = ReadCurrent(d, c, "A") // Phase A.
	values[1] = ReadCurrent(d, c, "B") // Phase B.
	values[2] = ReadCurrent(d, c, "C") // Phase C.
	values[3] = ReadCurrent(d, c, "N") // Phase N.

	// Measure Voltages.
	var measureVoltage1, measureVoltage2, measureVoltage3 bool
	values[4], measureVoltage1 = ReadVoltage(d, c, "A") // Phase A.
	values[5], measureVoltage2 = ReadVoltage(d, c, "B") // Phase B.
	values[6], measureVoltage3 = ReadVoltage(d, c, "C") // Phase C.

	// Measure Active Watts.
	if measureVoltage1 {
		values[7] = ReadActiveWatts(d, c, "A") // Phase A.
	} else {
		values[7] = values[0] * values[4]
	}
	if measureVoltage2 {
		values[8] = ReadActiveWatts(d, c, "B") // Phase B.
	} else {
		values[8] = values[1] * values[5]
	}
	if measureVoltage3 {
		values[9] = ReadActiveWatts(d, c, "C") // Phase C.
	} else {
		values[9] = values[2] * values[6]
	}

	// Measure cosphis.
	values[10] = ReadAngle(d, c, "A") // Phase A.
	values[11] = ReadAngle(d, c, "B") // Phase B.
	values[12] = ReadAngle(d, c, "C") // Phase C.

	// Measure frequencies.
	values[13] = ReadFrequency(d, c, "A") // Phase A.
	values[14] = ReadFrequency(d, c, "B") // Phase B.
	values[15] = ReadFrequency(d, c, "C") // Phase C.

	// Measure apparent power (volt-amps).
	values[16] = ReadApparentPower(d, c, "A") // Phase A.
	values[17] = ReadApparentPower(d, c, "B") // Phase B.
	values[18] = ReadApparentPower(d, c, "C") // Phase C.

	// Measure reactive power (volt-ampere reactive).
	values[19] = ReadReactivePower(d, c, "A") // Phase A.
	values[20] = ReadReactivePower(d, c, "B") // Phase B.
	values[21] = ReadReactivePower(d, c, "C") // Phase C.

	// Calculate power factors.
	values[22] = CalculatePowerFactor(c, "A", values[7], values[16], values[20]) // Phase A.
	values[23] = CalculatePowerFactor(c, "B", values[8], values[17], values[21]) // Phase B.
	values[24] = CalculatePowerFactor(c, "C", values[9], values[19], values[22]) // Phase C.

	values[25] = ReadActiveEnergy(d, c, "A") // Phase A.
	values[26] = ReadActiveEnergy(d, c, "B") // Phase B.
	values[27] = ReadActiveEnergy(d, c, "C") // Phase C.

	logLine := fmt.Sprintf("ReadValues: %s ", time.Since(startTime))
	logLine += fmt.Sprintf("I1: %g  I2: %g  I3: %g  I4: %g  ", values[0], values[1], values[2], values[3])
	logLine += fmt.Sprintf("V1: %g  V2: %g  V3: %g  ", values[4], values[5], values[6])
	logLine += fmt.Sprintf("P1: %g  P2: %g  P3: %g  ", values[7], values[8], values[9])
	logLine += fmt.Sprintf("COS1: %g  COS2: %g  COS3: %g  ", values[10], values[11], values[12])
	logLine += fmt.Sprintf("F1: %g  F2: %g  F3: %g  ", values[13], values[14], values[15])
	logLine += fmt.Sprintf("AVA: %g  BVA: %g  CVA: %g  ", values[16], values[17], values[18])
	logLine += fmt.Sprintf("AVAR: %g  BVAR: %g  CVAR: %g  ", values[19], values[20], values[21])
	logLine += fmt.Sprintf("PFA: %g  PFB: %g  PFC: %g  ", values[22], values[23], values[24])
	logLine += fmt.Sprintf("AWATTHR: %g  BWATTHR: %g  CWATTHR: %g  ", values[25], values[26], values[27])
	log.Debug(logLine)

	return values
}
