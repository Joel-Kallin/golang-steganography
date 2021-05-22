package main

import (
	"errors"
	"strconv"
)

func hideOneChar(byteIndex int, ch byte, image []byte) {
	writeIndex := byteIndex
	for j := 0; j < 4; j++ {
		orig := image[writeIndex]
		origWithTwoSmallestBitsZero := orig & 252 // orig & 111100
		twoSmallestBitsOfChar := ch & 3 // ch & 11
		byteWithHiddenBits := origWithTwoSmallestBitsZero | twoSmallestBitsOfChar
		image[writeIndex] = byteWithHiddenBits
		writeIndex ++
		ch = ch >> 2 // move the next two bits into position for next iteration
	}
}

func readOneChar(byteIndex int, image []byte)  (char byte) {
	char = 0
	readIndex := byteIndex
	for j := 0; j < 4; j++ {
		origWithSixBiggestBitsZero := image[readIndex] & 3 // orig & 000011
		origWithRelevantBitsShifted := origWithSixBiggestBitsZero << (j*2)
		char = char | origWithRelevantBitsShifted
		readIndex ++
	}
	return
}

func writeSecret(secret []byte, image []byte) error {
	var writeIndex = len(image)/2
	// All hidden messages begin with the length of the msg with a | to signal length termination and msg beginning
	var msgBeginningText = []byte(strconv.Itoa(len(secret))+"|")
	if len(image)/2-50 < (len(secret)*4)+(len(msgBeginningText)*4) {
		return errors.New("image to small to hide message")
	}
	for _, ch := range msgBeginningText {
		hideOneChar(writeIndex, ch, image)
		writeIndex += 4
	}

	for _, ch := range secret {
		hideOneChar(writeIndex, ch, image)
		writeIndex += 4
	}

	return nil
}

func readSecret(image []byte) (string, error) {

	var readIndex = len(image)/2
	var msgLength = ""
	for {
		ch := readOneChar(readIndex, image)
		readIndex += 4
		if ch=='|' {
			break
		}
		msgLength = msgLength + string(ch)
	}
	length, err := strconv.Atoi(msgLength)
	if err != nil {
		return "", errors.New("message in file incorrectly formatted or missing")
	}
	secret := make([]byte, length)


	for i := range secret {
		ch := readOneChar(readIndex, image)
		readIndex += 4
		secret[i] = ch
	}

	return string(secret), nil
}

func Encode(file []byte, message []byte) ([]byte, error) {
	encodedFile := file
	err := writeSecret(message, encodedFile)

	if err != nil {
		return nil, err
	}

	return encodedFile, nil
}

func Decode(file []byte) (string, error) {
	encodedFile := file
	return readSecret(encodedFile)
}
