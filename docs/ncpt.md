# Nasal Cannula Pressure Transducer (NCPT)

## Description

A nasal cannula pressure transducer is used in polysomnography to monitor 
airflow by measuring pressure changes during inspiration and expiration through
the nasal passages.

## Design

### Nasal Cannulas

**TODO**: Review different nasal cannula manufacturers.

### Pressure Transducers

#### MPS20N0040D

40 KPa MEMs pressure sensor.

MEMStek Co. Ltd - Obscure fabless semiconductor company.

Might be a generic name for a 40 KPa MEMs pressure sensor in China, with different manufacturers.

https://wenku.baidu.com/view/7306d761783e0912a2162a31.html

##### Results

* The MPS20N0040D has very poor performance at the low end of it's measurement range.

#### US9111-006

40 KPa MEMs pressure sensor.

http://www.unisense.com.tw/product%20info.php

##### Results

* Genuine parts perform well across the entire measurement range.
* Received counterfeit non-functional parts from AliExpress, so the supply chain is suspect.

#### XGZP010

10 KPa MEMs pressure sensor.

https://www.cfsensor.cn/proinfo/6.html

#### XGZP040 / GZP160-040S / XGZP160040S

40 KPa MEMs pressure sensor.

https://www.cfsensor.cn/proinfo/6.html

### Analog-to-Digital Converters

#### ADS1232

24-Bit, 80SPS, 2-Ch (Differential), Pin-Programmable Delta-Sigma ADC for Bridge Sensors, SPI Interface.

##### Results

* Prohibitively expensive.

#### CS1237

24-bit, 1280-SPS, single-channel, delta-sigma ADC with PGA, internal oscillator, temperature sensor, and SPI interface.

##### Results

* Performs very well.
* Custom serial interface, but not difficult to implement and only requires a 2-wires.
* Programmable gain, configurable conversion rates etc, quite a flexible device.
* Small PCB footprint.
* 640Hz mode is great for airflow monitoring.

#### HX710/711

24-Bit, 2-Channels, 80SPS/10SPS, Differential Input, Low Noise, Low Drift, ADC with PGA and Reference.

##### Results

* HX710x is too slow (40SPS max).

#### NAU7802

Low-power 24-bit ADC, with PGA, I2C interface, internal oscillator, and temperature sensor.

##### Results

* Performs well.
* Genuine I2C interface.
* Hum filter introduces some weird signal artifacts (it's a digital filter with some suspect coefficients).
* Often out of stock and hard to source with long lead times.