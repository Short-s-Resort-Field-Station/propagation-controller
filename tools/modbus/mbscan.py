import serial, struct, sys, time
PORT='/dev/serial/by-id/usb-FTDI_USB-RS485-WE_FTBCX8VV-if00-port0'
def crc(d):
    c=0xFFFF
    for b in d:
        c^=b
        for _ in range(8):
            c=(c>>1)^0xA001 if c&1 else c>>1
    return struct.pack('<H',c)
def req(s,addr,fc,reg,n,wait):
    f=struct.pack('>BBHH',addr,fc,reg,n); f+=crc(f)
    s.reset_input_buffer(); s.write(f); time.sleep(wait)
    r=s.read(64)
    if len(r)>=5 and r[0]==addr and r[1]==fc and crc(r[:-2])==r[-2:]: return r
    if len(r)>=5 and r[0]==addr and r[1]==fc|0x80: return r
    return None
bauds=[int(b) for b in sys.argv[1].split(',')] if len(sys.argv)>1 else [9600,4800,19200]
addrs=[int(a) for a in sys.argv[2].split(',')] if len(sys.argv)>2 else list(range(1,33))+[0xFF]
for baud in bauds:
    s=serial.Serial(PORT,baud,bytesize=8,parity='N',stopbits=1,timeout=0.15)
    wait=0.12 if baud>=9600 else 0.25
    for a in addrs:
        for fc in (3,4):
            r=req(s,a,fc,0,4,wait)
            if r: print(f"HIT baud={baud} addr={a} fc={fc} raw={r.hex()}"); sys.stdout.flush()
    s.close()
    print(f"scanned baud {baud}"); sys.stdout.flush()
