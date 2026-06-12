#!/usr/bin/env python3
"""
Docker Claude 봇에 사용자 계정으로 메시지 전송 및 응답 수신
Usage:
  python py/batch/tg_send.py <bot_username> <message>              # 전송만
  python py/batch/tg_send.py --ask <bot_username> <message>        # 전송 후 응답 대기
  python py/batch/tg_send.py --auth <phone> [2fa_password]         # 최초 인증
"""
import sys
import asyncio
import time
from pathlib import Path
import yaml
from telethon import TelegramClient

# config.yaml은 프로젝트 루트 (py/../config.yaml)
_cfg = yaml.safe_load((Path(__file__).parent.parent.parent / "config.yaml").read_text())["telegram"]
API_ID = _cfg["api_id"]
API_HASH = _cfg["api_hash"]
SESSION_FILE = _cfg["session_file"]

async def send_message(bot_username: str, message: str, retries: int = 3):
    for attempt in range(retries):
        try:
            async with TelegramClient(SESSION_FILE, API_ID, API_HASH) as client:
                await client.send_message(bot_username, message)
                print(f"[tg_send] {bot_username} 에게 메시지 전송 완료")
                return
        except Exception as e:
            if "database is locked" in str(e) and attempt < retries - 1:
                import time
                time.sleep(1 + attempt)
                continue
            raise

async def ask_message(bot_username: str, message: str, timeout: int = 300):
    """메시지 전송 후 응답 대기 (최대 timeout초)"""
    async with TelegramClient(SESSION_FILE, API_ID, API_HASH) as client:
        msgs = await client.get_messages(bot_username, limit=1)
        last_id = msgs[0].id if msgs else 0

        await client.send_message(bot_username, message)
        print(f"[tg_send] {bot_username} 에게 메시지 전송 완료, 응답 대기 중 (최대 {timeout}초)...", flush=True)

        deadline = time.time() + timeout
        prev_response = ""
        last_activity = time.time()

        while time.time() < deadline:
            await asyncio.sleep(3)
            new_msgs = await client.get_messages(bot_username, min_id=last_id, limit=50)
            bot_msgs = [m for m in reversed(new_msgs) if m.out is False and m.id > last_id]
            if bot_msgs:
                combined = "\n".join(m.text or "" for m in bot_msgs if m.text)
                if combined and combined != prev_response:
                    last_activity = time.time()
                    prev_response = combined
                    last_id = bot_msgs[-1].id
                elif combined and (time.time() - last_activity) > 5:
                    break

        if prev_response:
            print(f"\n[{bot_username} 응답]\n{prev_response}")
        else:
            print(f"[tg_send] 응답 없음 (timeout {timeout}초 초과)")

async def auth_only(phone: str, password: str = None):
    """최초 1회 인증용"""
    import getpass
    client = TelegramClient(SESSION_FILE, API_ID, API_HASH)
    pw = password or getpass.getpass("Telegram 2단계 인증 비밀번호: ")
    await client.start(phone=phone, password=pw)
    me = await client.get_me()
    print(f"[tg_send] 인증 완료: {me.first_name} (@{me.username})")
    await client.disconnect()

if __name__ == "__main__":
    if len(sys.argv) >= 2 and sys.argv[1] == "--auth":
        if len(sys.argv) < 3:
            print("Usage: tg_send.py --auth <phone_number> [2fa_password]")
            sys.exit(1)
        pw = sys.argv[3] if len(sys.argv) >= 4 else None
        asyncio.run(auth_only(sys.argv[2], pw))
    elif len(sys.argv) >= 2 and sys.argv[1] == "--ask":
        if len(sys.argv) < 4:
            print("Usage: tg_send.py --ask <bot_username> <message>")
            sys.exit(1)
        bot_username = sys.argv[2]
        message = " ".join(sys.argv[3:])
        asyncio.run(ask_message(bot_username, message))
    elif len(sys.argv) >= 3:
        bot_username = sys.argv[1]
        message = " ".join(sys.argv[2:])
        asyncio.run(send_message(bot_username, message))
    else:
        print("Usage:")
        print("  tg_send.py --auth <phone_number> [2fa]     # 최초 인증")
        print("  tg_send.py --ask <bot_username> <message>  # 전송 + 응답 수신")
        print("  tg_send.py <bot_username> <message>        # 전송만")
        sys.exit(1)
