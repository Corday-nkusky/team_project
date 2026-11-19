# 操作系统｜02 进程管理

进程（英语：process），是指计算机中已执行的程序，曾经是分时系统的基本运作单位。在面向进程设计的系统（如早期的UNIX，Linux 2.4及更早的版本）中，是程序的基本执行实体；在面向线程设计的系统（如当代多数操作系统、Linux 2.6及更新的版本）中，进程本身不是基本执行单位，而是线程的容器。
程序本身只是指令、数据及其组织形式的描述，相当于一个名词，进程才是程序（那些指令和数据）的真正执行实例，可以想像说是现在进行式。若干进程有可能与同一个程序相关系，且每个进程皆可以同步（循序）或异步（平行）的方式独立执行。现代计算机系统可在同一段时间内以进程的形式将多个程序加载到存储器中，并借由时间共享（或称时分复用），以在一个处理器上表现出同时（平行性）执行的感觉。同样的，使用多线程技术的操作系统或计算机体系结构，同样程序的平行线程，可在多CPU主机或网络上真正同时执行（在不同的CPU上）。


### 名称
整批系统环境，进程称为工作（jobs）。
分时系统环境，进程称为用户程序（user programs）或任务（tasks）。
在多数情况，工作与进程是同义词，但进程（process）已较为人接受


### 概念
用户下达执行程序的命令后，就会产生进程。同一程序可产生多个进程（一对多关系），以允许同时有多位用户执行同一程序，却不会相冲突。
进程需要一些资源才能完成工作，如CPU使用时间、存储器、文件以及I/O设备，且为依序逐一进行，也就是每个CPU核心任何时间内仅能执行一项进程。
进程与线程的区别：进程是计算机管理运进程序的一种方式，一个进程下可包含一个或者多个线程。线程可以理解为子进程。


### 内容
一个计算机系统进程包括（或者说“拥有”）下列资料：

那个程序的可执行机器代码的一个在存储器的映像。
分配到的存储器（通常是虚拟的一个存储器区域）。存储器的内容包括可执行代码、特定于进程的资料（输入、输出）、调用堆栈、堆栈（用于保存运行时运输中途产生的资料）。
分配给该进程的资源的操作系统描述符，诸如文件描述符（Unix术语）或文件句柄（Windows）、资料源和资料终端。
安全特性，诸如进程拥有者和进程的权限集（可以容许的操作）。
处理器状态（内文），诸如寄存器内容、物理存储器寻址等。当进程正在执行时，状态通常存储在寄存器，其他情况在存储器。


### 状态
进程在执行时，状态（state）会改变。所谓状态，就是指进程目前的动作：

新生（new）：进程新产生中。
执行（running）：正在执行。
等待（waiting）：等待某事发生，例如等待用户输入完成。亦称“阻塞”（blocked）
就绪（ready）：排班中，等待CPU。
结束（terminated）：完成执行。
各状态名称可能随不同操作系统而相异；对于单CPU系统（UP），任何时间可能有多个进程为等待、就绪，但必定仅有一个进程在执行。
注意: 进程的各个状态之间是不能随意切换的，例如当进程运行时因IO操作而阻塞，当IO操作完成后并不会直接恢复回运行态，而是转为就绪态等待CPU的调度。


### 进程控制块
### 线程
### 调度
### 进程间通信（Inter-process communication）
### Unix进程
### Windows进程
操作系统使用进程ID来唯一标识每个进程。在一个进程内部，使用进程句柄来标识关注的每个进程。使用Windows API从进程ID获取进程句柄：

使用API函数：GetModuleFileNameEx或GetProcessImageFileName或QueryFullProcessImageName查询进程的exe文件名
使用API函数GetCurrentProcess可以获取本进程的伪句柄（值为-1），只能用于本进程的API函数调用；不能被其他进程继承或复制。可用API函数DuplicateHandle获得进程的真句柄。
使用API函数CreateProcess创建进程，WaitForSingleObject可等待子进程的结束。例如：

创建的子进程可以继承父进程的：

CreateFile返回的打开句柄，包括文件、控制台输入缓冲区、控制台屏幕缓冲区, 命名管道, 串口通信设备, 邮槽.
打开的句柄，包括：进程、线程、互斥锁、事件对象、信号量、命名管道、匿名管道、文件映射对象。
环境变量
当前目录
控制台，除非进程脱离（detach）或创建了新的控制台。
错误模式，使用API函数SetErrorMode设置
进程亲和掩码（affinity mask），用以指示期望使用CPU的哪些核
在哪个作业中。
子进程不能继承：

优先级类别Priority class.
句柄，由LocalAlloc, GlobalAlloc, HeapCreate, HeapAlloc返回
伪句柄，由GetCurrentProcess或GetCurrentThread返回
DLL模块句柄，由LoadLibrary返回
GDI对象句柄或USER对象句柄，如HBITMAP或HMENU.
为继承句柄，父进程在创建（或者代开、复制）各种可继承对象句柄时，在SECURITY_ATTRIBUTES结构的bInheritHandle成员为TRUE。在CreateProcess的bInheritHandles参数为TRUE；如果要继承标准输入、标准输出、标准错误的句柄，STARTUPINFO结构的dwFlags成员包含STARTF_USESTDHANDLES标志位.
下述API的函数用于获取进程相关信息：

GetCommandLine：当前进程的命令行字符串
GetStartupInfo：当前进程被创建时的STARTUPINFO结构
GetProcessVersion：获取可执行头的版本信息
GetModuleFileName：获取包含了进程代码的可执行文件的全路径与文件名
GetGuiResources：获取使用中的GUI对象的句柄数量
IsDebuggerPresent：确定进程是否被调试
GetProcessIoCounters：获取进程执行的所有I/O操作的薄记信息。
GetProcessMemoryInfo：获取进程的工作集内存的信息
GetProcessWorkingSetSize：获取进程的工作集内存被允许的下限与上限
SetProcessWorkingSetSize：设置进程的工作集内存的下限与上限
进程终止时，所有打开的句柄被关闭，进程对象被触发（signaled）。进程的退出码（exit code）或者在ExitProcess、TerminateProcess函数中指出，或者是main、WinMain函数返回值。如果进程由于一个致命异常（fatal exception）而终止，退出码是这个异常值，同时进程的所有执行中的线程的退出码也是这个异常值。
优雅地关闭其他进程的方法是用RegisterWindowMessage登记私有消息，用BroadcastSystemMessage播放消息，收到消息的进程用ExitProcess关闭。
如果想要获取特定名字的进程的ID，需要枚举所有进程。传统办法是CreateToolhelp32Snapshot、Process32First、Process32Next函数；也可以使用EnumProcesses、EnumProcessModules函数来获取所有的进程ID，一个进程的所有模块的句柄。示例如下：


### 相关
时间片
进程树
