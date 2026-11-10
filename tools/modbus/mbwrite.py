import serial, struct, sys, time
PORT='/dev/serial/by-id/usb-FTDI_USB-RS485-WE_FTBCX8VV-if00-port0'
def crc(d):
    c=0xFFFF
    for b in d:
        c^=b
        for _ in range(8):
            c=(c>>1)^0xA001 if c&1 else c>>1
    return struct.pack('<H',c)
def rd(s,addr,reg,n):
    f=struct.pack('>BBHH',addr,3,reg,n); f+=crc(f)
    s.reset_input_buffer(); s.write(f); time.sleep(0.2); r=s.read(300)
    if len(r)>=5 and crc(r[:-2])==r[-2:] and r[1]==3:
        n=r[2]//2; return list(struct.unpack('>'+'H'*n, r[3:3+2*n]))
    return None
def wr(s,addr,reg,val):
    f=struct.pack('>BBHH',addr,6,reg,val); f+=crc(f)
    s.reset_input_buffer(); s.write(f); time.sleep(0.3); r=s.read(64)
    return r.hex()
step=sys.argv[1]
if step=='addr':
    s=serial.Serial(PORT,9600,timeout=0.3)
    print("before:", rd(s,1,0,8))
    print("write addr 1->6 reply:", wr(s,1,0,6))
    time.sleep(0.5)
    print("read @addr6 9600:", rd(s,6,0,8))
    print("read @addr1 9600:", rd(s,1,0,8))
elif step=='baud':
    s=serial.Serial(PORT,9600,timeout=0.3)
    print("before:", rd(s,6,0,8))
    print("write baud code 3->2 reply:", wr(s,6,1,2)); s.close()
    time.sleep(0.8)
    for b in (4800,9600,2400,1200,19200):
        s=serial.Serial(PORT,b,timeout=0.3); v=rd(s,6,0,8); s.close()
        print(f"read @addr6 {b}:", v)
        if v: break
