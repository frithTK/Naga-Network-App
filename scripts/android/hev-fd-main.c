int hev_socks5_tunnel_main(const char *config_path, int tun_fd);

int main(int argc, char **argv)
{
	if (argc < 2) {
		return 1;
	}
	return hev_socks5_tunnel_main(argv[1], 3);
}
