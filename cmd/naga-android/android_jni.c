#include <jni.h>
#include <stdlib.h>

extern char *NagaStart(char *, char *);
extern void NagaStop(void);
extern void NagaStopRuntime(void);
extern char *NagaSetTunFd(int);
extern void NagaCloseTun(void);

static jstring goErr(JNIEnv *env, char *err)
{
	if (err == NULL) {
		return (*env)->NewStringUTF(env, "");
	}
	jstring out = (*env)->NewStringUTF(env, err);
	free(err);
	return out;
}

JNIEXPORT jstring JNICALL
Java_eu_nagavpn_naga_1network_NagaControl_jniStart(JNIEnv *env, jclass cls, jstring dir, jstring nativeDir)
{
	const char *data = (*env)->GetStringUTFChars(env, dir, 0);
	const char *native = (*env)->GetStringUTFChars(env, nativeDir, 0);
	char *err = NagaStart((char *)data, (char *)native);
	(*env)->ReleaseStringUTFChars(env, dir, data);
	(*env)->ReleaseStringUTFChars(env, nativeDir, native);
	return goErr(env, err);
}

JNIEXPORT void JNICALL
Java_eu_nagavpn_naga_1network_NagaControl_jniStop(JNIEnv *env, jclass cls)
{
	NagaStop();
}

JNIEXPORT void JNICALL
Java_eu_nagavpn_naga_1network_NagaControl_jniStopRuntime(JNIEnv *env, jclass cls)
{
	NagaStopRuntime();
}

JNIEXPORT jstring JNICALL
Java_eu_nagavpn_naga_1network_NagaControl_jniSetTunFd(JNIEnv *env, jclass cls, jint fd)
{
	return goErr(env, NagaSetTunFd((int)fd));
}

JNIEXPORT void JNICALL
Java_eu_nagavpn_naga_1network_NagaControl_jniCloseTun(JNIEnv *env, jclass cls)
{
	NagaCloseTun();
}
