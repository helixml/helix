// TEMPORARY (task 003513): unit test for resolv_conf_scope_id() in
// qemu-utm's vendored libslirp. The helper is static inside the __APPLE__
// branch of slirp.c, so CI extracts it to helper.c and includes it here.
#include <arpa/inet.h>
#include <net/if.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static bool in6_equal(const struct in6_addr *a, const struct in6_addr *b)
{
    return memcmp(a, b, sizeof(*a)) == 0;
}

#include "helper.c"

static int fails;

static void check(const char *name, const char *conf, const char *addr,
                  uint32_t want)
{
    FILE *f = fopen("resolv.conf.test", "w");
    struct in6_addr a;
    uint32_t got;

    fputs(conf, f);
    fclose(f);
    inet_pton(AF_INET6, addr, &a);
    got = resolv_conf_scope_id("resolv.conf.test", &a);
    printf("%-26s got=%u want=%u %s\n", name, got, want,
           got == want ? "ok" : "FAIL");
    fails += got != want;
}

int main(void)
{
    uint32_t lo = if_nametoindex("lo0");
    struct in6_addr any = { 0 };

    if (lo == 0) {
        printf("FAIL: no lo0\n");
        return 1;
    }
    check("macOS hotspot resolv.conf",
          "#\n# macOS Notice\n#\nsearch .\n"
          "nameserver fe80::e0ba:78ff:fed0:9164%lo0\n",
          "fe80::e0ba:78ff:fed0:9164", lo);
    check("numeric scope, tab", "nameserver\tfe80::1%7\n", "fe80::1", 7);
    check("picks matching line",
          "nameserver 192.168.1.1\nnameserver fe80::2%lo0\n"
          "nameserver fe80::1%lo0\n", "fe80::1", lo);
    check("no scope in file", "nameserver fe80::1\n", "fe80::1", 0);
    check("address not listed", "nameserver fe80::1%lo0\n", "fe80::9", 0);
    check("empty file", "", "fe80::1", 0);
    check("unknown interface", "nameserver fe80::1%nosuch0\n", "fe80::1", 0);
    if (resolv_conf_scope_id("/nonexistent", &any) != 0) {
        printf("FAIL: missing file\n");
        fails++;
    }
    printf("%s\n", fails ? "FAILED" : "ALL PASSED");
    return fails != 0;
}
