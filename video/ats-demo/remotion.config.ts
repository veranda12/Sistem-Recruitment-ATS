import { Config } from "@remotion/cli/config";

// Still PNG agar teks UI tidak kabur saat dicek; video akhir H.264 yuv420p (lihat script render).
Config.setVideoImageFormat("png");
Config.setPixelFormat("yuv420p");
Config.setCodec("h264");
Config.setCrf(16);
