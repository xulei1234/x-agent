#!/usr/bin/env python
# -*- coding: utf-8 -*-
###################################################################
# File Name: user.py
# Author: bre_sa
# Created Time: 2019-12-25
# Description:
# 1. create_user
# 2. delete_user
###################################################################
import sys
import logging
import subprocess
from threading import Timer
import time
import os
import pwd

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)

fh = logging.FileHandler('/var/log/fortress_user.log', encoding='utf-8')
fh.setLevel(logging.DEBUG)
ch = logging.StreamHandler()
ch.setLevel(logging.DEBUG)

formatter = logging.Formatter('%(asctime)s - %(threadName)s - %(levelname)s - '
                              '%(lineno)s - %(message)s')
fh.setFormatter(formatter)
ch.setFormatter(formatter)

logger.addHandler(fh)
# logger.addHandler(ch)

VERSION = 2.3
PYTHEN_VERSION = sys.version[0:3]
PY_MAIN_VERSION = PYTHEN_VERSION[0]


# ===================== 工具函数 ============================
def timeout_process(process, show_command, timeout):
    """
    计时器超时时，执行的操作：
    1. 关掉进程
    2. 打印日志
    :param process:
    :param show_command:
    :param timeout:
    :return:
    """
    process.kill()
    msg = 'local command %s over %s, timeout' % (show_command, timeout)
    logger.error(msg)



def execute_local_command(command, show_command=None, timeout=30):
    """
    执行本地命令
    :param command: 运行的任务
    :param show_command: 脱敏后的命令，输出在日志中，方便调试
    :param timeout: 命令超时时间，默认15s
    :return: success(任务是否成功), output(标准输出), err_output(标准错误)
    """
    try:
        env = {
            "PATH": "/bin:/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin"
        }
        process = subprocess.Popen(command,
                                   stdout=subprocess.PIPE,
                                   stderr=subprocess.PIPE,
                                   env=env,
                                   shell=True)
        timer = Timer(timeout, timeout_process, [process, show_command, timeout])
        timer.start()
        if show_command is None:
            show_command = command
        logger.info("[start] to run local command: %s" % show_command)

        process.wait()

        returncode = process.returncode
        stdout = process.stdout.read().decode("u8")
        stderr = process.stderr.read().decode("u8")
        msg = u"[end] of running local command: %s  \n returncode: %s \n " \
                "stdout: %s \n stderr: %s \n " % (show_command, returncode,
                                                stdout, stderr)
        logger.info(msg)
        success = (process.returncode == 0)
        return success, stdout, stderr
    except Exception as e:
        logger.exception(e)
        raise
    finally:
        timer.cancel()

class UserHandler(object):

    def __init__(self, ):
        pass
    
    def create_user(self, user, group, key, password, is_bigdata_service_account):
        """
        Args:
            user: 用户名
            group: 用户组
            key: 用户公钥
            password: 用户密码
            is_bigdata_service_account: 是否是大数据平台服务账号, 0:(否), 1(是)
        备注: 如果是大数据平台服务账号， 用户不需要添加到组中
        """
        is_bigdata_service_account = int(is_bigdata_service_account)
        user_flag = self.is_user_exists(user)
        group_flag = self.is_group_exists(group)
        logger.debug("user_flag: %s, group_flag: %s", user_flag, group_flag)
        assert is_bigdata_service_account or group_flag, (40, "group: %s not exists" % group)
        # 检查 /devops 目录是否存在
        if not os.path.exists("/devops"):
            self.exec_common_cmd("mkdir /devops") 
        if not user_flag:
            create_cmd = "useradd -d /devops/{user} -m {user} -s /bin/bash".format(user=user)
            self.create_user_step(create_cmd, "useradd", user, group, key)
        # 检查 /devops/{user} 目录是否存在
        home_path = "/devops/" + user
        if not os.path.exists(home_path):
            self.exec_common_cmd("mkdir %s" % home_path)
        # 检查 .bashrc .bash_logout .bash_profile(centos) .profile(ubuntu)
        bashrc_path = home_path + "/" + ".bashrc"
        source_bash_path = "/etc/skel/."
        if not os.path.exists(bashrc_path):
            self.exec_common_cmd("/bin/cp -rf %s %s" % (source_bash_path, home_path))

        # 公钥覆盖写入
        ssh_cmd = """
            mkdir -p /devops/{user}/.ssh
            echo  {key} > /devops/{user}/.ssh/authorized_keys
            chmod 600 /devops/{user}/.ssh/authorized_keys
        """.format(
            user=user,
            key=key
        )
        self.create_user_step(ssh_cmd, "ssh-rsa write", user, group, key)

        # 目录授权
        chown_cmd = "chown -R {user}:{user} /devops/{user}".format(user=user)
        self.create_user_step(chown_cmd, "chown", user, group, key)

        # 添加用户到组
        if not is_bigdata_service_account:
            usermod_cmd = "usermod -d /devops/{user} -aG {group} -m {user} ".format(
                user=user,
                group=group
            )
            self.create_user_step(usermod_cmd, "add user to group", user, group, key)
        
        # 修改用户密码，解决用户密码过期的问题，同时也修改默认用户douyuops
        for each_user in [user, "douyuops"]:
            user_change_passwd_cmd = 'echo {user}:{password} | chpasswd'.format(
                password=password,
                user=each_user
            )
            self.create_user_step(user_change_passwd_cmd, "change user password", each_user, group, key)
        
        return True

    def delete_user(self, user):
        """
        Args:
            user: 用户名
        """
        del_cmd = """
            userdel -r {user}
            rm -rf /devops/{user}
        """.format(user=user)
        success, stdout, stderr = execute_local_command(del_cmd)
        err_msg = u"delete user: {user} failed, stdout: {stdout}, stderr: {stderr}".format(
            user=user,
            stdout=stdout,
            stderr=stderr
        )
        assert success, (40, err_msg)
        return True

    def update_user_group(self, user, group):
        """
        Args:
            user: 用户名
            group: 用户组,多个用户组用逗号(,)分隔
        """
        update_user_group_cmd = "usermod {user} -G {group}".format(
            user=user,
            group=group
        )
        success, stdout, stderr = execute_local_command(update_user_group_cmd)
        err_msg = u"update_user_group, user: {user} failed, stdout: {stdout}, stderr: {stderr}".format(
            user=user,
            stdout=stdout,
            stderr=stderr
        )
        assert success, (40, err_msg)
        return True


    def exec_common_cmd(self, shell_cmd):
        success, stdout, stderr = execute_local_command(shell_cmd)
        err_msg = u"shell_cmd: {shell_cmd} failed, stdout: {stdout}, stderr: {stderr}".format(
            shell_cmd=shell_cmd,
            stdout=stdout,
            stderr=stderr
        )
        assert success, (40, err_msg)
        return True

    def is_user_exists(self, user):
        user_list = [item.pw_name for item in pwd.getpwall()]
        if user in user_list:
            return True
        else:
            return False
    
    def is_group_exists(self, group):
        shell_cmd = "grep -i {group}  /etc/group |wc -l".format(group=group)
        success, stdout, stderr = execute_local_command(shell_cmd)
        assert success, (40, "check group exists failed")
        if stdout.strip() == '0':
            return False
        else:
            return True
    
    def create_user_step(self, shell_cmd, desc, user, group, key):
        """
        Args:
            shell_cmd: shell脚本执行命令
            desc: 执行描述
        """
        success, stdout, stderr = execute_local_command(shell_cmd)
        err_msg = u"""
            {desc} failed, user: {user}, group: {group}, key: {key}, 
            stdout: {stdout}, stderr: {stderr}
        """.format(
            desc=desc,
            user=user,
            group=group,
            key=key,
            stdout=stdout,
            stderr=stderr
        )
        assert success, (40, err_msg)


if __name__ == "__main__":

    code = 0
    stderr = ""
    stdout = "success"

    warn_msg = \
        "version: %s \n" % VERSION + \
        "python_version: %s \n" % PYTHEN_VERSION + \
        "usage: \n" + \
        "    python user.py create_user user group key password is_bigdata_service_account \n" + \
        "    or python user.py delete_user user \n" + \
        "    or python user.py update_user_group user group\n"

    logger.debug("sys.argv: %s", sys.argv)
    if len(sys.argv) < 3:
        code = 40
        stdout = ""
        stderr = warn_msg
    else:
        try:
            function_name = sys.argv[1]
            params = sys.argv[2:]
            user_handler = UserHandler()
            function = getattr(user_handler, function_name, None)
            assert function, (40, "only create_user or delete_user or update_user_group is allowed")
            function(*params)
        except AssertionError as exc:
            logger.exception(exc)
            if PYTHEN_VERSION <= "2.6":
                code = exc.args[0]
                stderr = exc.args[1]
            elif PY_MAIN_VERSION == '2':
                code = exc.message[0]
                stderr = exc.message[1]
            else:
                code = exc.args[0][0]
                stderr = exc.args[0][1]
            stdout = ""
        except Exception as exc:
            logger.exception(exc)
            code = 50
            if PY_MAIN_VERSION == '2':
                stderr = exc.message
            else:
                stderr = exc.args[0]
            stdout = ""
    
    # 写入标准输出
    sys.stdout.write(stdout)
    sys.stderr.write(stderr)
    exit(code)
