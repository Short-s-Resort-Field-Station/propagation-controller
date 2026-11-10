import serial, struct, sys, time
PORT='/dev/serial/by-id/usb-FTDI_USB-RS485-WE_FTBCX8VV-if00-port0'
baud=int(sys.argv[1]); addr=int(sys.argv[2]); secs=int(sys.argv[3])
def crc(d):
    c=0xFFFF
    for b in d:
        c^=b
        for _ in range(8):
            c=(c>>1)^0xA001 if c&1 else c>>1
    return struct.pack('<H',c)
s=serial.Serial(PORT,baud,timeout=0.2)
def rd(fc,reg,n):
    f=struct.pack('>BBHH',addr,fc,reg,n); f+=crc(f)
    s.reset_input_buffer(); s.write(f); time.sleep(0.15); r=s.read(300)
    if len(r)>=5 and crc(r[:-2])==r[-2:] and r[1]==fc:
        n=r[2]//2; return list(struct.unpack('>'+'H'*n, r[3:3+2*n]))
    return None
t0=time.time()
while time.time()-t0<secs:
    v=rd(3,0,8)
    print(f"{time.time()-t0:5.1f}s regs0-7={v}"); sys.stdout.flush(); time.sleep(1)
