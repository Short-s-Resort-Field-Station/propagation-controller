import serial, struct, sys, time
PORT='/dev/serial/by-id/usb-FTDI_USB-RS485-WE_FTBCX8VV-if00-port0'
def crc(d):
    c=0xFFFF
    for b in d:
        c^=b
        for _ in range(8):
            c=(c>>1)^0xA001 if c&1 else c>>1
    return struct.pack('<H',c)
s=serial.Serial(PORT,9600,timeout=0.2)
def rd(addr,fc,reg,n):
    f=struct.pack('>BBHH',addr,fc,reg,n); f+=crc(f)
    s.reset_input_buffer(); s.write(f); time.sleep(0.15); r=s.read(300)
    if len(r)>=5 and r[1]==fc|0x80: return f"EXC{r[2]}"
    if len(r)>=5 and crc(r[:-2])==r[-2:] and r[1]==fc:
        n=r[2]//2; return list(struct.unpack('>'+'H'*n, r[3:3+2*n]))
    return None
for fc in (4,3):
    for base in (0,16,32,64,100,0x100,0x200,0x1000,0x2000):
        v=rd(1,fc,base,8)
        if v not in (None,) : print(f"fc{fc} @{base:#06x}: {v if isinstance(v,str) else [hex(x) for x in v]}")
print("--- fc4 regs 0-3, five reads 1 s apart (watch for change):")
for i in range(5):
    print(rd(1,4,0,4)); time.sleep(1)
