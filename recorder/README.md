# OpenPSG Recorder

The OpenPSG Recorder is a Linux application that records PSG data from one or
more Ethernet sensors and saves it to an EDF file.

## Running

### Granting NET_ADMIN privileges

`NET_ADMIN` capabilities are required to configure network interface and bind
to low ports. To grant the required capabilities to the recorder, run the
following command:

```shell
sudo setcap 'cap_net_admin+ep cap_net_bind_service+ep' ./recorder
```