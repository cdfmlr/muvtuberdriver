package main

import (
	"testing"
)

func Test_tooLong(t *testing.T) {
	type args struct {
		text     string
		maxWords int
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "tooLongLatin",
			args: args{
				text:     "hello my name is foo bar",
				maxWords: 5,
			},
			want: true,
		},
		{
			name: "tooLongCJK",
			args: args{
				text:     "一二三四五六",
				maxWords: 5,
			},
			want: true,
		},
		{
			name: "tooLongCJKWithLatin",
			args: args{
				text:     "一二三 hello world 五",
				maxWords: 5,
			},
			want: true,
		},
		{
			name: "notTooLonglatin",
			args: args{
				text:     "hello world one two three",
				maxWords: 5,
			},
			want: false,
		},
		{
			name: "notTooLongCJK",
			args: args{
				text:     "一二三四五",
				maxWords: 5,
			},
			want: false,
		},
		{
			name: "notTooLongCJKWithLatin",
			args: args{
				text:     "一二 hello world",
				maxWords: 5,
			},
			want: false,
		},
		{
			name: "notTooLongCJKWithLatinAndPunctuation",
			args: args{
				text:     "一二, hello",
				maxWords: 5,
			},
			want: false,
		},
		{
			name: "notTooLongEvidently",
			args: args{
				text:     "一w",
				maxWords: 5,
			},
			want: false,
		},
		{
			name: "emptyString",
			args: args{
				text:     "",
				maxWords: 5,
			},
			want: false,
		},
		{
			name: "maxWordsNegative",
			args: args{
				text:     "hello",
				maxWords: -1,
			},
			want: true,
		},
		{
			name: "maxWordsZero",
			args: args{
				text:     "hello",
				maxWords: 0,
			},
			want: true,
		},
		{
			name: "emptyStringWithMaxWordsZero",
			args: args{
				text:     "",
				maxWords: 0,
			},
			want: false,
		},
		{
			name: "emptyStringWithMaxWordsEmpty",
			args: args{
				text:     "",
				maxWords: -1,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tooLong(tt.args.text, tt.args.maxWords); got != tt.want {
				t.Errorf("tooLong() = %v, want %v", got, tt.want)
			}
		})
	}
}
