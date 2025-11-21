#include <stdio.h>
#include <unistd.h>

int main()
{
    printf("hello , this a test container!\n");
    int tick=0;
    while(1){
        printf("uptime %d\n",tick);
        fflush(stdout);
        sleep(1);
        tick++;
    }
    return 0;
}
